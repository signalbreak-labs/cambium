# Bounded XSD patterns

`Compile` parses one complete XML Schema regular expression. Matching covers the
whole string; `^` and `$` in the source are literal characters. Decimal counts
have arbitrary magnitude and leading zeros have their numerical meaning. Unicode
categories, XML name escapes, block escapes, and class subtraction reuse Cambium's
character semantics. Cambium also preserves its existing named ASCII class
extension, such as `[[:alnum:]_]*`, with the classes and complements documented
by [Go regexp/syntax](https://pkg.go.dev/regexp/syntax). These are compatibility
syntax, not additional XSD character escapes. XSD subtraction keeps precedence
after a nonempty base: `[a-[:digit:]]` subtracts the literal `:digit:` character
group, whereas `[[:digit:]]` is the ASCII digit extension. A leading raw dash
before a named class is literal, as in `[-[:digit:]]`. A standalone raw dash
is allowed at either end of a positive group or, for compatibility, immediately
after a completed range: `[a-b-c]` matches `a`, `b`, `c`, or `-`. This post-range
case extends the literal [XSD 1.0 character-range rule](https://www.w3.org/TR/xmlschema-2/#char-ranges);
it does not change YANG's normative XSD version or admit arbitrary Go/PCRE syntax.
Raw dashes are never range endpoints; escaped dashes may be. Subtraction retains
precedence, so `[a-z-[aeiou]]` excludes vowels, while `[a-z--[aeiou]]` also
includes a literal dash.

The alphabet consists of Unicode scalar values. As with Go's
regexp package, invalid UTF-8 bytes in a candidate are read as replacement runes.

```go
program, err := yangpattern.Compile(`(a|aa){01001}`)
if err != nil { return err }
matched, err := program.MatchString(value)
if err != nil { return err }
// Apply a YANG invert-match modifier only here, after successful evaluation.
```

A `Program` is immutable and safe for concurrent matching. `Export` returns a deep
copy; `Restore` validates and deep-copies its input, including unreachable nodes.
A zero-value program is invalid. `ConfigurationError` reports invalid source or
metadata. `ResourceError` reports an unresolved operation, with `Phase`, `Stage`,
`Limit`, `Allowance`, and `Observed`; it is not a mismatch. Wrapped errors retain
these types. Synchronous native matching does not promise cancellation.

The portable format has an explicit `FormatVersion`, ordered `Nodes`, and a
`Root` index. Empty language, epsilon, and class nodes have no children;
alternation and concatenation have two ordered children; repetition has one
nonnullable child. Only classes have sorted, nonoverlapping scalar `Ranges`.
Only repetition has canonical decimal `Min`/`Max` strings. An unbounded upper
limit uses `Unbounded: true` and empty `Max`. Zero is `"0"`; a finite upper bound
must be at least the lower bound. Source spelling is diagnostic and is never
reparsed by the runtime. No policy or native-engine handle is serialized. Constructors reserve work for validating and comparing decimal
bounds, then build an immutable saturating numeric cache in a separately charged
pass. Matching, including native preparation and fallback, reads this
cache without rescanning decimal strings. The cache is never exported and is
recomputed from the actual supplied bounds on every restore.

The compiler removes nullable repeat children, combines exact nested repetition
and intervals starting at zero, and preserves nested intervals with holes. Match
specialization clips counts against candidate length without changing exported
metadata. The dispatcher uses direct repeated-class scans, eligible canonical Go
regexps, a sparse derivative DAG, then bounded native expansion. Ordinary and
expanded native execution share instruction/source/work checks. Cached native
state never waives a stricter per-call policy. No value-dependent arena survives
a call, and no package-level value cache is maintained.

`CompileWithLimits`, `RestoreWithLimits`, and `MatchStringWithLimits` are separate
policies. Zero fields choose defaults, never unlimited work. The defaults are:

| Phase | Limits |
| --- | --- |
| Compile | 256 KiB source; 32,768 nodes; 65,536 edges; depth 256; 65,536 decimal digits; 131,072 ranges; 16 MiB accounted storage; 5,000,000 work units |
| Restore | 32,768 nodes; 65,536 edges; depth 256; 65,536 digits; 131,072 ranges; 16 MiB DTO/copy storage; 5,000,000 work units |
| Match | 32,768 residual nodes; 131,072 transitions; 16 MiB accounted arena/storage; 5,000,000 structural work units; 16,384 native instructions; 256 KiB native source; 200,000,000 native instruction × (runes + 1) units |

The derivative evaluator also bounds recursive normalization to 512 active calls.
These are operational ceilings, not restrictions on legal candidate length.
Direct scanning and stable transition-cache hits do not spend structural work
per rune. Storage accounting is conservative, checked before allocations, and
separate from the allocator measurements in the engineering acceptance tests.
Optional native preparation may be omitted when constructor resources are low.
Each evaluator attempt has its declared allowance; exhausting speculative
matching does not prevent an eligible bounded fallback from proving a result.

`Program.GoRegexp` is a separate translation API. It emits canonical, anchored Go
syntax or an error, including for counts that Go cannot represent natively.
`cambium.Pattern.Program` exposes this compiler while preserving `Regex()` and
separate inversion. Schema-default, data-tree, and codegen consumers are being
migrated separately; this package alone does not imply those consumers already
use the bounded runtime.

`RuntimeSources()` returns the explicit ordered source inventory and fresh byte
slices. Copy these files byte-for-byte, including their package declaration and
Apache-2.0 attribution, into a separate `yangpattern` package. They use only the
Go 1.26 standard library and expose restoration/matching without the source
compiler. Preserve the Apache-2.0 license in downstream distributions. Generators
can emit `ProgramData` literals; no second parser or textual-regexp transport is
required.

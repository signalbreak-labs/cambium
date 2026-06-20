# Building the Go cgo engine (libyang + PCRE2)

The Go layer links the **same vendored libyang + PCRE2** as the Rust layer
(`/third_party`), built statically so `go build` needs no system libyang.

## Recipe

```sh
bash go/internal/libyang/build.sh
```

This runs the identical two-stage CMake build as
`rust/cambium-libyang-sys/build.rs` (same flags → byte-identical engine, per
`spec/ordering-invariants.md` §4):

1. **PCRE2** → static, `-fPIC`, `PCRE2_BUILD_PCRE2_8=ON`, into
   `.build/pcre2-install`.
2. **libyang** → static, `-fPIC`, `ENABLE_LYD_PRIV=OFF`, linked against the
   staged PCRE2, into `.build/libyang-install`.

`cgo` then references these via `${SRCDIR}/.build/...` in `libyang.go`
(`CFLAGS` for the headers, `LDFLAGS` linking `libyang.a` then `libpcre2-8.a`
then `-lm -lpthread`, plus `-ldl` on Linux). Static archives are passed by
path, so there is no dynamic `-lyang`/`-lpcre2` dependency.

`.build/` is git-ignored; rerun `build.sh` after a clean checkout or a
`/third_party` bump. The script is idempotent (skips if the archives exist).

## Platform status (v0.1)

| Platform | Status |
|---|---|
| macOS arm64 | supported — `-lm -lpthread` resolve via libSystem |
| Linux x86_64 (glibc) | supported — same recipe |
| Linux x86_64/arm64 (musl, static) | Supported via `zig cc`. Set `CAMBIUM_CC="zig cc -target x86_64-linux-musl"` before running `build.sh`; `build.sh` will write a wrapper so CMake treats it as a single compiler. Then build the Go module with `CGO_ENABLED=1 CC="zig cc -target x86_64-linux-musl" GOOS=linux GOARCH=amd64 go build -ldflags '-extldflags "-static"' ./...`. A known-good zig is 0.13+; CI pins it. |
| Windows/MSVC | post-1.0 (needs pcre2/pthreads-win32/dlfcn-win32 vcpkg shims) |

## Release flattening

`go get` does not clone git submodules, so the vendored C must be copied into
the module before publish. Run `bash scripts/release-flatten.sh` from the repo
root; this creates `go/internal/libyang/vendor/{libyang,pcre2}`. Verify with
`bash scripts/check-flattened-build.sh`, which builds the Go module in
isolation using only the flattened C. See `PUBLISHING.md`.

## Known follow-ups

- Richer FFI error mapping: pull `ly_errmsg`/`ly_err_first` and surface the
  `CAMBIUM_xxxx` rule code (currently the adapter returns the libyang `rc`).

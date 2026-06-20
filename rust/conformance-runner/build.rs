use std::path::PathBuf;

// cambium-libyang-sys builds a static libyang + the `yanglint` CLI into its
// OUT_DIR. That path is exposed to the sys crate's own compilation via
// `cargo:rustc-env=CAMBIUM_YANGLINT`, but `cargo:rustc-env` does NOT propagate
// to dependent crates, so the conformance runner's `option_env!` was always
// None and the yanglint oracle silently never ran. Locate yanglint relative to
// our own OUT_DIR (sibling build dirs under target/<profile>/build/) and
// re-expose it for this crate so the oracle is actually wired.
fn main() {
    println!("cargo:rerun-if-changed=build.rs");
    let Ok(out) = std::env::var("OUT_DIR") else {
        return;
    };
    // OUT_DIR = target/<profile>/build/conformance-runner-<hash>/out
    let build_root = PathBuf::from(&out).join("..").join("..");
    if let Ok(entries) = std::fs::read_dir(&build_root) {
        for entry in entries.flatten() {
            let candidate = entry.path().join("out/libyang-install/bin/yanglint");
            if candidate.exists() {
                println!("cargo:rustc-env=CAMBIUM_YANGLINT={}", candidate.display());
                return;
            }
        }
    }
}

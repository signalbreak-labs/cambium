use std::env;
use std::path::{Path, PathBuf};

fn main() {
    println!("cargo:rerun-if-changed=build.rs");
    println!("cargo:rerun-if-changed=src/wrappers.c");

    let manifest_dir = PathBuf::from(env::var_os("CARGO_MANIFEST_DIR").unwrap());
    let out_dir = PathBuf::from(env::var_os("OUT_DIR").unwrap());

    if env::var_os("DOCS_RS").is_some() {
        // docs.rs has no network and may not have CMake. Copy the committed
        // pre-generated bindings into OUT_DIR so the crate can compile without
        // building any C code.
        copy_committed_bindings(
            &manifest_dir,
            &out_dir,
            "docs.rs build: using pre-generated bindings, no C compilation",
        );
        return;
    }

    // Release-flatten layout: vendor/ inside the crate takes precedence over the
    // workspace third_party/ tree. This lets the published crate carry its own C
    // source (submodules are not cloned by `cargo fetch` or `go get`).
    let pcre2_src = manifest_dir.join("vendor/pcre2");
    let libyang_src = manifest_dir.join("vendor/libyang");
    let (pcre2_src, libyang_src) = if pcre2_src.exists() && libyang_src.exists() {
        (pcre2_src, libyang_src)
    } else {
        let root_dir = manifest_dir.ancestors().nth(2).unwrap().to_path_buf();
        (
            root_dir.join("third_party/pcre2"),
            root_dir.join("third_party/libyang"),
        )
    };

    let pcre2_build = out_dir.join("pcre2-build");
    let pcre2_install = out_dir.join("pcre2-install");
    let libyang_build = out_dir.join("libyang-build");
    let libyang_install = out_dir.join("libyang-install");

    let target = env::var("TARGET").unwrap();
    let profile = env::var("PROFILE").unwrap();
    let is_debug = profile == "debug";

    // Stage 1: build static PCRE2 with -fPIC.
    let mut pcre2_cfg = cmake::Config::new(&pcre2_src);
    pcre2_cfg
        .out_dir(&pcre2_build)
        .define("BUILD_SHARED_LIBS", "OFF")
        .define("PCRE2_BUILD_PCRE2_8", "ON")
        .define("PCRE2_BUILD_PCRE2_16", "OFF")
        .define("PCRE2_BUILD_PCRE2_32", "OFF")
        .define("PCRE2_BUILD_PCRE2GREP", "OFF")
        .define("PCRE2_BUILD_TESTS", "OFF")
        .define("CMAKE_POSITION_INDEPENDENT_CODE", "ON")
        .define("CMAKE_INSTALL_PREFIX", &pcre2_install)
        .define(
            "CMAKE_BUILD_TYPE",
            if is_debug { "Debug" } else { "Release" },
        );

    let pcre2_out = pcre2_cfg.build();

    // PCRE2 may install into the configured prefix or into the cmake out dir.
    let pcre2_prefix = if pcre2_install.exists() {
        pcre2_install
    } else {
        pcre2_out
    };
    let pcre2_lib = find_library_dir(&pcre2_prefix, &["libpcre2-8.a", "pcre2-8.lib"])
        .expect("static pcre2-8 library directory not found");
    let pcre2_include = pcre2_prefix.join("include");

    let pcre2_lib_file = find_pcre2_lib(&pcre2_lib).expect("static pcre2-8 library not found");

    // Stage 2: build static libyang against the staged PCRE2.
    let mut libyang_cfg = cmake::Config::new(&libyang_src);
    libyang_cfg
        .out_dir(&libyang_build)
        .define("BUILD_SHARED_LIBS", "OFF")
        .define("CMAKE_POSITION_INDEPENDENT_CODE", "ON")
        .define("ENABLE_LYD_PRIV", "OFF")
        .define("ENABLE_TESTS", "OFF")
        .define("PCRE2_LIBRARY", pcre2_lib_file)
        .define("PCRE2_INCLUDE_DIR", pcre2_include)
        .define("CMAKE_INSTALL_PREFIX", &libyang_install)
        .define(
            "CMAKE_BUILD_TYPE",
            if is_debug { "Debug" } else { "Release" },
        );

    // libyang may need help finding its own bundled/internal deps; keep it self-contained.
    if target.contains("apple") {
        // No extra system libs required for the core static build.
    }

    let libyang_out = libyang_cfg.build();

    let libyang_prefix = if libyang_install.exists() {
        libyang_install
    } else {
        libyang_out
    };
    let libyang_lib = find_library_dir(&libyang_prefix, &["libyang.a", "yang.lib"])
        .expect("static libyang library directory not found");
    let libyang_include = libyang_prefix.join("include/libyang");

    // Expose the built yanglint binary path so the conformance runner can use it
    // as an independent oracle. Missing on a docs.rs/no-CMake build is fine.
    let yanglint = libyang_prefix.join("bin/yanglint");
    if yanglint.exists() {
        println!(
            "cargo:rustc-env=CAMBIUM_YANGLINT={}",
            yanglint.canonicalize().unwrap_or(yanglint).display()
        );
    }

    // Tell cargo where to link.
    println!("cargo:rustc-link-search=native={}", libyang_lib.display());
    println!("cargo:rustc-link-search=native={}", pcre2_lib.display());
    println!("cargo:rustc-link-lib=static=yang");
    println!("cargo:rustc-link-lib=static=pcre2-8");

    // libyang depends on pthreads and math.
    println!("cargo:rustc-link-lib=pthread");
    println!("cargo:rustc-link-lib=m");

    if target.contains("apple") {
        // CoreFoundation / libiconv are not required for the basic build.
    }

    compile_wrappers(&manifest_dir, &libyang_include);

    // Generate bindings with bindgen.
    generate_bindings(&libyang_include, &out_dir, &manifest_dir);
}

fn compile_wrappers(manifest_dir: &Path, libyang_include: &Path) {
    let mut build = cc::Build::new();
    build.file(manifest_dir.join("src/wrappers.c"));
    build.include(libyang_include);
    if let Some(include_root) = libyang_include.parent() {
        build.include(include_root);
    }
    build.compile("cambium_libyang_wrappers");
}

fn find_pcre2_lib(dir: &Path) -> Option<PathBuf> {
    for name in ["libpcre2-8.a", "pcre2-8.lib"] {
        let candidate = dir.join(name);
        if candidate.exists() {
            return Some(candidate);
        }
    }
    None
}

fn find_library_dir(prefix: &Path, names: &[&str]) -> Option<PathBuf> {
    for dir_name in ["lib", "lib64"] {
        let dir = prefix.join(dir_name);
        if names.iter().any(|name| dir.join(name).exists()) {
            return Some(dir);
        }
    }
    None
}

fn copy_committed_bindings(manifest_dir: &Path, out_dir: &Path, reason: &str) {
    let committed_bindings = manifest_dir.join("src/bindings.rs");
    let out_bindings = out_dir.join("bindings.rs");
    std::fs::copy(&committed_bindings, &out_bindings)
        .expect("committed bindings.rs must exist for fallback build");
    println!("cargo:warning={reason}");
}

fn generate_bindings(include_dir: &Path, out_dir: &Path, manifest_dir: &Path) {
    use std::process::Command;

    let header = include_dir.join("libyang.h");
    let output = out_dir.join("bindings.rs");

    // Use bindgen CLI if available so we do not need bindgen as a library
    // dependency in the default build. This keeps DOCS_RS builds simple.
    let status = Command::new("bindgen")
        .arg(&header)
        .arg("--output")
        .arg(&output)
        .arg("--allowlist-function")
        .arg("ly.*")
        .arg("--allowlist-type")
        .arg("ly.*")
        .arg("--allowlist-var")
        .arg("LY.*")
        .arg("--default-enum-style")
        .arg("rust")
        .arg("--no-doc-comments")
        .arg("--generate")
        .arg("functions,types,vars")
        .arg("--")
        .arg("-I")
        .arg(include_dir)
        .status();

    match status {
        Ok(s) if s.success() => {}
        Ok(s) => panic!("bindgen failed with status: {s}"),
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => copy_committed_bindings(
            manifest_dir,
            out_dir,
            "bindgen CLI not found: using committed pre-generated bindings",
        ),
        Err(err) => panic!("bindgen failed: {err}"),
    }

    println!("cargo:rerun-if-changed={}", header.display());
}

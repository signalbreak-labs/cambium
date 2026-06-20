//! Integration test: the conformance runner passes the whole corpus.

use std::process::Command;

#[test]
fn conformance_runner_passes() {
    let status = match Command::new(env!("CARGO_BIN_EXE_conformance-runner")).status() {
        Ok(s) => s,
        Err(e) => panic!("failed to start conformance-runner: {e}"),
    };
    assert!(
        status.success(),
        "conformance-runner exited with {status:?}"
    );
}

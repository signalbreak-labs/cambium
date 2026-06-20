//! Error types for the Cambium safe core.

use std::fmt;
use std::path::PathBuf;

/// Result type used throughout Cambium.
pub type Result<T> = std::result::Result<T, Error>;

/// A stable Cambium diagnostic code (see `spec/rule-codes.md`). The same failure
/// yields the same code in Rust and Go.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum RuleCode {
    /// Unclassified internal error (`CAMBIUM_E0000`).
    Unknown,
    /// Context/schema setup error (`CAMBIUM_E0001`).
    Context,
    /// Data parse error (`CAMBIUM_E0002`).
    Parse,
    /// RFC-7950 validation error (`CAMBIUM_E0003`).
    Validate,
    /// Serialization error (`CAMBIUM_E0004`).
    Serialize,
    /// Path/positional list-operation error (`CAMBIUM_E0005`).
    OrderedList,
    /// Data-tree access/mutation by path or XPath failed (`CAMBIUM_E0006`).
    DataPath,
    /// Data handle used after an invalidating mutation (`CAMBIUM_E0007`).
    Stale,
}

impl RuleCode {
    /// The `CAMBIUM_E####` string form.
    pub fn as_str(self) -> &'static str {
        match self {
            RuleCode::Unknown => "CAMBIUM_E0000",
            RuleCode::Context => "CAMBIUM_E0001",
            RuleCode::Parse => "CAMBIUM_E0002",
            RuleCode::Validate => "CAMBIUM_E0003",
            RuleCode::Serialize => "CAMBIUM_E0004",
            RuleCode::OrderedList => "CAMBIUM_E0005",
            RuleCode::DataPath => "CAMBIUM_E0006",
            RuleCode::Stale => "CAMBIUM_E0007",
        }
    }
}

impl fmt::Display for RuleCode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

/// The kind of error as classified by the protocol or application layer.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ErrorType {
    /// An application-level validation or data error.
    Application,
    /// A transport/protocol-level error.
    Protocol,
}

/// A sub-code derived from a validation failure, finer-grained than `RuleCode`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[non_exhaustive]
pub enum ValidationCode {
    /// A `must` constraint was violated.
    Must,
    /// A `when` condition was not satisfied.
    When,
    /// A `mandatory`, `min-elements`, `max-elements`, or choice constraint was
    /// not satisfied.
    Mandatory,
    /// A `leafref` or `instance-identifier` could not be resolved.
    Leafref,
    /// A value violates a type or semantic data restriction.
    InvalidValue,
}

/// One structured validation diagnostic.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Diagnostic {
    /// Stable top-level rule code (always `RuleCode::Validate` here).
    pub code: RuleCode,
    /// Human-readable message from the engine.
    pub message: String,
    /// Data instance path, if available.
    pub data_path: Option<String>,
    /// Schema path, if available.
    pub schema_path: Option<String>,
    /// Application or protocol classification.
    pub error_type: ErrorType,
    /// RFC 8040 / RFC 7950 error-app-tag, if provided.
    pub error_app_tag: Option<String>,
    /// Fine-grained validation sub-code, when it can be inferred.
    pub validation_code: Option<ValidationCode>,
}

/// A collection of validation diagnostics returned by `DataTree::validate`.
#[derive(Debug)]
pub struct ValidationErrors(Vec<Diagnostic>);

impl ValidationErrors {
    /// Create a non-empty diagnostic set.
    pub(crate) fn new(diagnostics: Vec<Diagnostic>) -> Self {
        Self(diagnostics)
    }

    /// Number of diagnostics.
    pub fn len(&self) -> usize {
        self.0.len()
    }

    /// True if the validation produced no diagnostics.
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    /// Iterate over the diagnostics in engine order.
    pub fn iter(&self) -> impl Iterator<Item = &Diagnostic> {
        self.0.iter()
    }

    /// The first diagnostic, treated as the primary failure.
    pub fn primary(&self) -> Option<&Diagnostic> {
        self.0.first()
    }

    /// All diagnostics as a slice.
    pub fn as_slice(&self) -> &[Diagnostic] {
        &self.0
    }
}

impl fmt::Display for ValidationErrors {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "validation failed with {} diagnostic(s)", self.0.len())
    }
}

impl std::error::Error for ValidationErrors {}

/// Errors returned by the Cambium safe core.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// An FFI-level operation failed.
    #[error("[{code}] {message}")]
    Engine {
        /// Stable diagnostic code.
        code: RuleCode,
        /// Human-readable description of what failed.
        message: String,
        /// Underlying cause, when one is available.
        #[source]
        source: Option<Box<dyn std::error::Error + Send + Sync>>,
    },
    /// One or more RFC-7950 validation constraints were violated.
    #[error("[{}] validation failed", RuleCode::Validate)]
    Validation(#[source] ValidationErrors),
    /// A path could not be converted to a C string.
    #[error("[{0}] invalid path: {path}", RuleCode::DataPath)]
    InvalidPath {
        /// The offending path.
        path: PathBuf,
    },
    /// A data handle was used after an invalidating mutation.
    #[error("[{0}] stale data handle", RuleCode::Stale)]
    Stale,
    /// Input contained an interior NUL byte.
    #[error("[{}] interior NUL byte", RuleCode::Parse)]
    Nul(#[from] std::ffi::NulError),
    /// Output was not valid UTF-8.
    #[error("[{0}] {source}", RuleCode::Parse)]
    Utf8 {
        /// The UTF-8 decoding failure.
        #[from]
        source: std::string::FromUtf8Error,
    },
}

impl Error {
    /// Construct an FFI error with an explicit rule code.
    pub fn ffi(code: RuleCode, message: impl Into<String>) -> Self {
        Error::Engine {
            code,
            message: message.into(),
            source: None,
        }
    }

    /// The stable diagnostic code for this error.
    pub fn rule_code(&self) -> RuleCode {
        match self {
            Error::Engine { code, .. } => *code,
            Error::Validation(_) => RuleCode::Validate,
            Error::InvalidPath { .. } => RuleCode::DataPath,
            Error::Stale => RuleCode::Stale,
            Error::Nul(_) | Error::Utf8 { .. } => RuleCode::Parse,
        }
    }
}

impl From<std::io::Error> for Error {
    fn from(e: std::io::Error) -> Self {
        Error::ffi(RuleCode::Unknown, format!("io error: {e}"))
    }
}

impl From<String> for Error {
    fn from(message: String) -> Self {
        Error::ffi(RuleCode::Unknown, message)
    }
}

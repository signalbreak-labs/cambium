//! Regression: `Decimal64` must render in RFC-7950 canonical form — trailing
//! fractional zeros stripped (keeping >= 1 digit) — and must NOT emit a double
//! minus for a negative value whose magnitude is >= 1 (the `whole` part is taken
//! as `unsigned_abs`, with the sign applied exactly once).
#![allow(clippy::unwrap_used)]

use cambium_core::Decimal64;

#[test]
fn decimal64_display_canonical() {
    let cases: &[(i64, u8, &str)] = &[
        (123400, 4, "12.34"), // trailing zeros stripped
        (1230, 2, "12.3"),
        (1200, 2, "12.0"), // keep at least one fractional digit
        (1234, 2, "12.34"),
        (0, 2, "0.0"),
        (-1234, 2, "-12.34"), // negative, magnitude >= 1 — no double minus
        (-12340, 3, "-12.34"),
        (-1500, 3, "-1.5"),
        (-50, 2, "-0.5"), // negative, magnitude < 1
        (-5, 3, "-0.005"),
    ];
    for &(raw, fd, want) in cases {
        assert_eq!(
            Decimal64::new(raw, fd).to_string(),
            want,
            "raw={raw} fraction_digits={fd}"
        );
    }
}

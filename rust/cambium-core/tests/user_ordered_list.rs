//! Functional + compile-fail guard for `UserOrderedList`.

use std::fs;
use std::path::PathBuf;

use cambium_core::{Context, Format, ParseMode, Result, SerializeFlags};

fn fixture_dir() -> Result<PathBuf> {
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .ancestors()
        .nth(2)
        .ok_or_else(|| cambium_core::Error::from("failed to locate fixture dir".to_string()))?
        .join("conformance/fixtures/ordered-user");
    Ok(dir)
}

#[test]
fn user_ordered_list_positional_mutations() -> Result<()> {
    let mut ctx = Context::new()?;
    ctx.set_search_path(fixture_dir()?.join("module"))?;
    ctx.load_module("ordered-user-demo")?;

    let input = fs::read_to_string(fixture_dir()?.join("input.xml"))?;
    let mut tree = ctx.parse(Format::Xml, ParseMode::data_only(), input.as_bytes())?;

    {
        let mut list = tree.user_ordered_list_at("/ordered-user-demo:config/entry[name='c']")?;
        // Initial order: c(0), a(1), b(2). Move a before c -> a, c, b.
        list.move_before(1, 0)?;
    }

    let output = tree.serialize(
        Format::Xml,
        SerializeFlags {
            siblings: true,
            ..Default::default()
        },
    )?;
    let output = String::from_utf8(output)?;

    let expected = r#"<config xmlns="urn:ordered-user-demo">
  <entry>
    <name>a</name>
    <value>1</value>
  </entry>
  <entry>
    <name>c</name>
    <value>3</value>
  </entry>
  <entry>
    <name>b</name>
    <value>2</value>
  </entry>
</config>"#;

    assert_eq!(output.trim(), expected.trim());
    Ok(())
}

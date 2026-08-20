//! Result types and human/machine-readable reporting for conformance checks.

use serde::Serialize;

/// Outcome of a single conformance check.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Status {
    Pass,
    Fail,
    Skip,
}

impl Status {
    fn as_str(self) -> &'static str {
        match self {
            Status::Pass => "PASS",
            Status::Fail => "FAIL",
            Status::Skip => "SKIP",
        }
    }
}

/// The result of running a single named conformance check.
#[derive(Debug, Clone, Serialize)]
pub struct CheckResult {
    pub name: String,
    pub status: Status,
    pub detail: String,
}

impl CheckResult {
    pub fn pass(name: impl Into<String>, detail: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            status: Status::Pass,
            detail: detail.into(),
        }
    }

    pub fn fail(name: impl Into<String>, detail: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            status: Status::Fail,
            detail: detail.into(),
        }
    }

    pub fn skip(name: impl Into<String>, detail: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            status: Status::Skip,
            detail: detail.into(),
        }
    }
}

/// Prints results as a human-readable, column-aligned table.
pub fn print_table(results: &[CheckResult]) {
    let name_width = results
        .iter()
        .map(|r| r.name.len())
        .max()
        .unwrap_or(4)
        .max("CHECK".len());

    println!("{:<name_width$}  STATUS  DETAIL", "CHECK", name_width = name_width);
    println!("{}", "-".repeat(name_width + 8 + 6));
    for result in results {
        // Keep the detail on one line for table output; replace newlines with " | ".
        let detail = result.detail.replace('\n', " | ");
        println!(
            "{:<name_width$}  {:<6}  {}",
            result.name,
            result.status.as_str(),
            detail,
            name_width = name_width
        );
    }

    let pass = results.iter().filter(|r| r.status == Status::Pass).count();
    let fail = results.iter().filter(|r| r.status == Status::Fail).count();
    let skip = results.iter().filter(|r| r.status == Status::Skip).count();
    println!();
    println!("{pass} passed, {fail} failed, {skip} skipped ({} total)", results.len());
}

/// Prints results as a JSON array of `{name, status, detail}` objects.
pub fn print_json(results: &[CheckResult]) {
    match serde_json::to_string_pretty(results) {
        Ok(json) => println!("{json}"),
        Err(e) => eprintln!("failed to serialize results as JSON: {e}"),
    }
}

/// Exit code: 0 iff no check has status `Fail` (skips are OK).
pub fn exit_code(results: &[CheckResult]) -> i32 {
    if results.iter().any(|r| r.status == Status::Fail) {
        1
    } else {
        0
    }
}

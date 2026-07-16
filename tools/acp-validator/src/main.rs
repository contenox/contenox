//! acp-validator: a conformance-checking ACP client used to validate
//! ACP Agent (server-side) implementations.
//!
//! Spawns the agent given by `--agent` over stdio and runs an ordered series
//! of independent conformance checks against it, reporting PASS/FAIL/SKIP
//! for each. Exits 0 iff no check FAILED.

mod checks;
mod report;

use std::collections::HashSet;
use std::time::Duration;

use clap::Parser;

/// Default trigger prompt: ask the `testy` reference agent (or any agent
/// that understands this JSON convention) to run its `callbacks` scenario,
/// which requests permission and performs `fs/write_text_file` followed by
/// `fs/read_text_file`. Agents that don't understand this JSON will just
/// treat it as an opaque text prompt, and the permission/fs checks that
/// depend on it will report SKIP rather than FAIL.
const DEFAULT_CALLBACKS_TRIGGER: &str = r#"{"command":"run_scenario","scenario":"callbacks"}"#;

/// Default trigger prompt for checks that need the agent to stream
/// `session/update` notifications.
const DEFAULT_STREAMING_TRIGGER: &str = r#"{"command":"run_scenario","scenario":"session_updates"}"#;

#[derive(Parser, Debug)]
#[command(
    name = "acp-validator",
    about = "Conformance-checking ACP client for validating ACP Agent implementations",
    version
)]
struct Cli {
    /// The command used to spawn the agent under test (e.g. "python my_agent.py"),
    /// or a JSON stdio server config understood by `AcpAgent::from_str`.
    #[arg(long)]
    agent: String,

    /// Comma-separated list of checks to run. Defaults to all checks.
    /// Available: initialize, version_negotiation, session_new,
    /// session_new_additional_directories, prompt_streaming,
    /// permission_roundtrip, fs_callbacks, cancel, set_mode, auth,
    /// update_ordering, unknown_method
    #[arg(long, value_delimiter = ',')]
    checks: Option<Vec<String>>,

    /// Emit machine-readable JSON instead of a human-readable table.
    #[arg(long)]
    json: bool,

    /// Per-check timeout, in seconds.
    #[arg(long, default_value_t = 15)]
    timeout: u64,

    /// Prompt text used to trigger a `session/request_permission` call for
    /// the permission round-trip check (check 6) and, by default, for the
    /// cancel-during-prompt check (check 8) too.
    #[arg(long, default_value = DEFAULT_CALLBACKS_TRIGGER)]
    permission_trigger: String,

    /// Prompt text used to trigger `fs/read_text_file` / `fs/write_text_file`
    /// calls for the filesystem callback check (check 7). Defaults to the
    /// same trigger as `--permission-trigger`.
    #[arg(long)]
    fs_trigger: Option<String>,

    /// Prompt text used to trigger a (typically multi-step) operation for
    /// the cancel-during-prompt check (check 8). Defaults to the same
    /// trigger as `--permission-trigger`.
    #[arg(long)]
    cancel_trigger: Option<String>,

    /// Prompt text used to trigger streaming `session/update` notifications
    /// for the prompt-streaming (check 5) and update-ordering (check 11)
    /// checks.
    #[arg(long, default_value = DEFAULT_STREAMING_TRIGGER)]
    streaming_trigger: String,
}

#[tokio::main]
async fn main() -> std::process::ExitCode {
    if std::env::var_os("RUST_LOG").is_some() {
        use tracing_subscriber::{EnvFilter, layer::SubscriberExt, util::SubscriberInitExt};
        tracing_subscriber::registry()
            .with(EnvFilter::from_default_env())
            .with(tracing_subscriber::fmt::layer().with_writer(std::io::stderr))
            .init();
    }

    let cli = Cli::parse();

    let enabled = cli.checks.as_ref().map(|names| {
        names
            .iter()
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
            .collect::<HashSet<String>>()
    });

    let config = checks::Config {
        agent_command: cli.agent.clone(),
        timeout: Duration::from_secs(cli.timeout),
        permission_trigger: cli.permission_trigger.clone(),
        fs_trigger: cli.fs_trigger.clone().unwrap_or_else(|| cli.permission_trigger.clone()),
        cancel_trigger: cli.cancel_trigger.clone().unwrap_or_else(|| cli.permission_trigger.clone()),
        streaming_trigger: cli.streaming_trigger.clone(),
        enabled,
    };

    let results = checks::run_all(&config).await;

    if cli.json {
        report::print_json(&results);
    } else {
        report::print_table(&results);
    }

    std::process::ExitCode::from(report::exit_code(&results) as u8)
}

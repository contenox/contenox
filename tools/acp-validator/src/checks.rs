//! Conformance checks exercised against an ACP Agent.
//!
//! Each check is an independent probe against the Agent under test: it never
//! aborts the overall run on failure, reports PASS/FAIL/SKIP with a short
//! detail string, and is bounded by a timeout so a hung or misbehaving Agent
//! can never make the validator hang.

use std::collections::HashMap;
use std::path::PathBuf;
use std::str::FromStr as _;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Mutex;
use std::time::Duration;

use agent_client_protocol::schema::ProtocolVersion;
use agent_client_protocol::schema::v1::{
    AuthenticateRequest, CancelNotification, ClientCapabilities, ContentBlock,
    CreateTerminalRequest, FileSystemCapabilities, Implementation, InitializeRequest,
    InitializeResponse, KillTerminalRequest, LogoutRequest, NewSessionRequest, PromptRequest,
    ReadTextFileRequest, ReadTextFileResponse, ReleaseTerminalRequest, RequestPermissionRequest,
    RequestPermissionOutcome, RequestPermissionResponse, SelectedPermissionOutcome, SessionId,
    SessionNotification, SessionUpdate, SetSessionModeRequest, StopReason, TerminalOutputRequest,
    TextContent, WaitForTerminalExitRequest, WriteTextFileRequest, WriteTextFileResponse,
};
use agent_client_protocol::{
    AcpAgent, Agent, Client, ConnectionTo, ErrorCode, Responder, UntypedMessage,
};

use crate::report::CheckResult;

/// CLI-derived configuration threaded through the check functions.
#[derive(Debug, Clone)]
pub struct Config {
    pub agent_command: String,
    pub timeout: Duration,
    pub permission_trigger: String,
    pub fs_trigger: String,
    pub cancel_trigger: String,
    pub streaming_trigger: String,
    pub enabled: Option<std::collections::HashSet<String>>,
}

impl Config {
    fn enabled(&self, name: &str) -> bool {
        match &self.enabled {
            None => true,
            Some(set) => set.contains(name),
        }
    }
}

/// How the shared `RequestPermissionRequest` handler should respond.
#[derive(Debug, Clone, Copy, Default)]
enum PermissionMode {
    /// Auto-select the first offered permission option (the "happy path").
    #[default]
    AutoSelectFirst,
    /// Auto-respond with the `Cancelled` outcome (robustness probe).
    AutoCancelled,
    /// Stash the responder for the driving check to answer later.
    Defer,
}

/// State shared between the background message handlers (registered once on
/// the `Builder`) and the sequential check driver running inside
/// `connect_with`.
#[derive(Default)]
struct Shared {
    /// All `session/update` notifications observed, tagged with a
    /// monotonically increasing sequence number assigned at receipt time.
    updates: Mutex<Vec<(u64, SessionNotification)>>,
    seq: AtomicU64,

    permission_mode: Mutex<PermissionMode>,
    permission_seen: AtomicBool,
    permission_last: Mutex<Option<RequestPermissionRequest>>,
    permission_pending: Mutex<Option<Responder<RequestPermissionResponse>>>,

    fs_files: Mutex<HashMap<PathBuf, String>>,
    fs_write_seen: AtomicBool,
    fs_read_seen: AtomicBool,
    fs_last_write: Mutex<Option<WriteTextFileRequest>>,
    fs_last_read: Mutex<Option<ReadTextFileRequest>>,
    fs_shape_violations: Mutex<Vec<String>>,
}

impl Shared {
    fn drain_updates(&self) -> Vec<(u64, SessionNotification)> {
        std::mem::take(&mut self.updates.lock().unwrap())
    }

    fn record_fs_write(&self, req: &WriteTextFileRequest) {
        self.fs_write_seen.store(true, Ordering::SeqCst);
        self.fs_files
            .lock()
            .unwrap()
            .insert(req.path.clone(), req.content.clone());
        *self.fs_last_write.lock().unwrap() = Some(req.clone());
    }

    fn record_fs_read(&self, req: &ReadTextFileRequest) -> String {
        self.fs_read_seen.store(true, Ordering::SeqCst);
        *self.fs_last_read.lock().unwrap() = Some(req.clone());
        self.fs_files
            .lock()
            .unwrap()
            .get(&req.path)
            .cloned()
            .unwrap_or_else(|| "acp-validator placeholder content\n".to_string())
    }
}

/// Runs a future with a timeout, translating both outcomes into a single
/// `Result<T, String>` describing what went wrong.
async fn with_timeout<T, F>(dur: Duration, label: &str, fut: F) -> Result<T, String>
where
    F: std::future::Future<Output = Result<T, agent_client_protocol::Error>>,
{
    match tokio::time::timeout(dur, fut).await {
        Ok(Ok(value)) => Ok(value),
        Ok(Err(e)) => Err(format!("{label} returned an error: {e}")),
        Err(_) => Err(format!("{label} timed out after {dur:?}")),
    }
}

/// Polls `predicate` every 20ms until it returns `true` or `dur` elapses.
/// Returns whether the predicate became true.
async fn wait_until(dur: Duration, mut predicate: impl FnMut() -> bool) -> bool {
    let deadline = tokio::time::Instant::now() + dur;
    loop {
        if predicate() {
            return true;
        }
        if tokio::time::Instant::now() >= deadline {
            return false;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
}

/// Error returned for any `terminal/*` request, since acp-validator never
/// advertises the `terminal` client capability.
fn terminal_not_supported() -> agent_client_protocol::Error {
    agent_client_protocol::util::internal_error(
        "acp-validator does not advertise the terminal client capability",
    )
}

fn client_capabilities() -> ClientCapabilities {
    ClientCapabilities::new().fs(
        FileSystemCapabilities::new()
            .read_text_file(true)
            .write_text_file(true),
    )
}

/// Runs every enabled check and returns the full set of results.
///
/// Two ACP connections (i.e. two Agent subprocesses) are used:
/// - A "primary" connection that runs all checks except version negotiation,
///   sharing a single session where that's meaningful.
/// - A throwaway connection dedicated to the version-negotiation edge case,
///   because it must send its own `initialize` with a bogus protocol version
///   as the very first message on a fresh connection.
pub async fn run_all(config: &Config) -> Vec<CheckResult> {
    let mut results = Vec::new();

    if config.enabled("version_negotiation") {
        results.push(run_version_negotiation_check(config).await);
    }

    match run_primary_checks(config).await {
        Ok(primary_results) => results.extend(primary_results),
        Err(e) => {
            results.push(CheckResult::fail(
                "connection",
                format!("primary connection failed: {e}"),
            ));
        }
    }

    results
}

/// Spawns a fresh Agent subprocess and drives the full primary check
/// sequence over a single connection/session.
async fn run_primary_checks(config: &Config) -> anyhow::Result<Vec<CheckResult>> {
    let agent = AcpAgent::from_str(&config.agent_command)?;
    let shared = std::sync::Arc::new(Shared::default());

    let results = Client
        .builder()
        .name("acp-validator")
        .on_receive_notification(
            {
                let shared = shared.clone();
                async move |notif: SessionNotification, _cx| {
                    let seq = shared.seq.fetch_add(1, Ordering::SeqCst);
                    shared.updates.lock().unwrap().push((seq, notif));
                    Ok(())
                }
            },
            agent_client_protocol::on_receive_notification!(),
        )
        .on_receive_request(
            {
                let shared = shared.clone();
                async move |request: RequestPermissionRequest, responder, _cx| {
                    shared.permission_seen.store(true, Ordering::SeqCst);
                    *shared.permission_last.lock().unwrap() = Some(request.clone());
                    let mode = *shared.permission_mode.lock().unwrap();
                    match mode {
                        PermissionMode::AutoSelectFirst => {
                            match request.options.first().map(|o| o.option_id.clone()) {
                                Some(id) => responder.respond(RequestPermissionResponse::new(
                                    RequestPermissionOutcome::Selected(
                                        SelectedPermissionOutcome::new(id),
                                    ),
                                )),
                                None => responder.respond(RequestPermissionResponse::new(
                                    RequestPermissionOutcome::Cancelled,
                                )),
                            }
                        }
                        PermissionMode::AutoCancelled => responder.respond(
                            RequestPermissionResponse::new(RequestPermissionOutcome::Cancelled),
                        ),
                        PermissionMode::Defer => {
                            *shared.permission_pending.lock().unwrap() = Some(responder);
                            Ok(())
                        }
                    }
                }
            },
            agent_client_protocol::on_receive_request!(),
        )
        .on_receive_request(
            {
                let shared = shared.clone();
                async move |request: WriteTextFileRequest, responder, _cx| {
                    if !request.path.is_absolute() {
                        shared.fs_shape_violations.lock().unwrap().push(format!(
                            "fs/write_text_file path is not absolute: {}",
                            request.path.display()
                        ));
                    }
                    shared.record_fs_write(&request);
                    responder.respond(WriteTextFileResponse::new())
                }
            },
            agent_client_protocol::on_receive_request!(),
        )
        .on_receive_request(
            {
                let shared = shared.clone();
                async move |request: ReadTextFileRequest, responder, _cx| {
                    if !request.path.is_absolute() {
                        shared.fs_shape_violations.lock().unwrap().push(format!(
                            "fs/read_text_file path is not absolute: {}",
                            request.path.display()
                        ));
                    }
                    let content = shared.record_fs_read(&request);
                    responder.respond(ReadTextFileResponse::new(content))
                }
            },
            agent_client_protocol::on_receive_request!(),
        )
        // acp-validator never advertises the `terminal` client capability, but some agents
        // (deliberately, in the case of `testy`'s `callbacks` scenario) still attempt
        // `terminal/*` calls unconditionally. We must still answer promptly with an error
        // instead of leaving the request unhandled, or a well-behaved agent that awaits the
        // response inline would hang the whole prompt turn.
        .on_receive_request(
            async move |_request: CreateTerminalRequest, responder, _cx| {
                responder.respond_with_error(terminal_not_supported())
            },
            agent_client_protocol::on_receive_request!(),
        )
        .on_receive_request(
            async move |_request: TerminalOutputRequest, responder, _cx| {
                responder.respond_with_error(terminal_not_supported())
            },
            agent_client_protocol::on_receive_request!(),
        )
        .on_receive_request(
            async move |_request: WaitForTerminalExitRequest, responder, _cx| {
                responder.respond_with_error(terminal_not_supported())
            },
            agent_client_protocol::on_receive_request!(),
        )
        .on_receive_request(
            async move |_request: KillTerminalRequest, responder, _cx| {
                responder.respond_with_error(terminal_not_supported())
            },
            agent_client_protocol::on_receive_request!(),
        )
        .on_receive_request(
            async move |_request: ReleaseTerminalRequest, responder, _cx| {
                responder.respond_with_error(terminal_not_supported())
            },
            agent_client_protocol::on_receive_request!(),
        )
        // Catch-all fallback, tried only after every specific handler above has declined.
        //
        // Without this, an agent-to-client request/notification carrying a `sessionId` that
        // we don't have a specific handler for (e.g. an unstable/experimental method like
        // `elicitation/create`) gets queued by the SDK's role-default handler with
        // `retry: true`, on the assumption that a per-session dynamic handler (as registered
        // by the higher-level session-builder API, which this validator intentionally
        // bypasses in favor of raw requests) will eventually claim it. Since we never install
        // one, that message would otherwise sit in the retry queue forever and hang whatever
        // agent-side call is awaiting our response. Answering immediately with "method not
        // found" here keeps every prompt turn responsive regardless of which extra callbacks
        // an agent decides to make.
        .on_receive_request(
            async move |request: UntypedMessage, responder, _cx| {
                responder.respond_with_error(
                    agent_client_protocol::Error::method_not_found().data(request.method),
                )
            },
            agent_client_protocol::on_receive_request!(),
        )
        .on_receive_notification(
            async move |_request: UntypedMessage, _cx| Ok(()),
            agent_client_protocol::on_receive_notification!(),
        )
        .connect_with(agent, move |cx| {
            let shared = shared.clone();
            let config = config.clone();
            async move { Ok(drive_primary_checks(cx, &config, &shared).await) }
        })
        .await?;

    Ok(results)
}

/// The actual sequential check driver, run as the `main_fn` of a single
/// `connect_with` call. Each step is independent: a failure only skips
/// steps that strictly require its output (e.g. everything requires
/// `initialize` to have produced a response; session-scoped checks require
/// a session id).
async fn drive_primary_checks(
    cx: ConnectionTo<Agent>,
    config: &Config,
    shared: &Shared,
) -> Vec<CheckResult> {
    let mut results = Vec::new();
    let timeout = config.timeout;

    // --- 1. initialize -------------------------------------------------------
    // `initialize` and `session/new` are prerequisites for almost every other
    // check, so they always run even if `--checks` didn't name them
    // explicitly; `--checks` only controls whether their own result row is
    // reported.
    let init_outcome = with_timeout(
        timeout,
        "initialize",
        cx.send_request(
            InitializeRequest::new(ProtocolVersion::V1)
                .client_capabilities(client_capabilities())
                .client_info(Implementation::new("acp-validator", env!("CARGO_PKG_VERSION"))),
        )
        .block_task(),
    )
    .await;
    if config.enabled("initialize") {
        match &init_outcome {
            Ok(response) => results.push(check_initialize_response(response)),
            Err(e) => results.push(CheckResult::fail("initialize", e.clone())),
        }
    }
    let Ok(init_response) = init_outcome else {
        if config.enabled("session_new") {
            results.push(CheckResult::skip(
                "session_new",
                "initialize did not complete; cannot create a session",
            ));
        }
        return results;
    };

    // --- 3. session/new ----------------------------------------------------
    let cwd = std::env::current_dir().unwrap_or_else(|_| PathBuf::from("/"));
    let session_outcome = with_timeout(
        timeout,
        "session/new",
        cx.send_request(NewSessionRequest::new(cwd.clone())).block_task(),
    )
    .await;
    if config.enabled("session_new") {
        match &session_outcome {
            Ok(response) => results.push(CheckResult::pass(
                "session_new",
                format!(
                    "session created: {} (modes: {}, config_options: {})",
                    response.session_id.0,
                    response.modes.is_some(),
                    response.config_options.as_ref().map(Vec::len).unwrap_or(0)
                ),
            )),
            Err(e) => results.push(CheckResult::fail("session_new", e.clone())),
        }
    }
    let session_id = session_outcome
        .ok()
        .map(|response| (response.session_id, response.modes));

    // --- 4. session/new with additionalDirectories -------------------------
    if config.enabled("session_new_additional_directories") {
        results.push(
            check_additional_directories(&cx, &init_response, &cwd, timeout).await,
        );
    }

    let Some((session_id, session_modes)) = session_id else {
        for name in [
            "prompt_streaming",
            "permission_roundtrip",
            "fs_callbacks",
            "cancel",
            "set_mode",
            "update_ordering",
            "unknown_method",
        ] {
            if config.enabled(name) {
                results.push(CheckResult::skip(name, "no session available"));
            }
        }
        if config.enabled("auth") {
            results.push(check_auth(&cx, &init_response, timeout).await);
        }
        return results;
    };

    // --- 5. session/prompt streaming ---------------------------------------
    if config.enabled("prompt_streaming") {
        results.push(
            check_prompt_streaming(&cx, shared, &session_id, &config.streaming_trigger, timeout)
                .await,
        );
    }

    // --- 6. permission round-trip -------------------------------------------
    if config.enabled("permission_roundtrip") {
        results.push(
            check_permission_roundtrip(
                &cx,
                shared,
                &session_id,
                &config.permission_trigger,
                timeout,
            )
            .await,
        );
    }

    // --- 7. fs callbacks -----------------------------------------------------
    if config.enabled("fs_callbacks") {
        results.push(
            check_fs_callbacks(&cx, shared, &session_id, &config.fs_trigger, timeout).await,
        );
    }

    // --- 9. session/set_mode ---------------------------------------------
    if config.enabled("set_mode") {
        results.push(check_set_mode(&cx, shared, &session_id, session_modes, timeout).await);
    }

    // --- 11. session/update ordering --------------------------------------
    if config.enabled("update_ordering") {
        results.push(
            check_update_ordering(&cx, shared, &session_id, &config.streaming_trigger, timeout)
                .await,
        );
    }

    // --- 8. session/cancel during prompt ------------------------------------
    if config.enabled("cancel") {
        results.push(
            check_cancel(&cx, shared, &session_id, &config.cancel_trigger, timeout).await,
        );
    }

    // --- 10. authenticate/logout --------------------------------------------
    if config.enabled("auth") {
        results.push(check_auth(&cx, &init_response, timeout).await);
    }

    // --- 12. unknown-method tolerance ----------------------------------------
    if config.enabled("unknown_method") {
        results.push(check_unknown_method(&cx, &session_id, timeout).await);
    }

    results
}

fn check_initialize_response(response: &InitializeResponse) -> CheckResult {
    if response.protocol_version.as_u16() != ProtocolVersion::V1.as_u16() {
        return CheckResult::fail(
            "initialize",
            format!(
                "requested protocolVersion=1 but agent responded with protocolVersion={}; \
                 per the initialization spec the Agent MUST echo the requested version if it \
                 supports it",
                response.protocol_version
            ),
        );
    }

    let caps = &response.agent_capabilities;
    let agent_info = response
        .agent_info
        .as_ref()
        .map(|info| format!("{} {}", info.name, info.version))
        .unwrap_or_else(|| "<absent, optional>".to_string());

    CheckResult::pass(
        "initialize",
        format!(
            "protocolVersion=1 negotiated; agentInfo={agent_info}; \
             loadSession={}; auth_methods={}; mcp(http={},sse={}); \
             session(list={},delete={},additionalDirectories={},resume={},close={})",
            caps.load_session,
            response.auth_methods.len(),
            caps.mcp_capabilities.http,
            caps.mcp_capabilities.sse,
            caps.session_capabilities.list.is_some(),
            caps.session_capabilities.delete.is_some(),
            caps.session_capabilities.additional_directories.is_some(),
            caps.session_capabilities.resume.is_some(),
            caps.session_capabilities.close.is_some(),
        ),
    )
}

/// Check 2: a second, independent connection sends `initialize` with an
/// absurd protocol version (999). Per the spec, the Agent MUST NOT error and
/// MUST NOT echo back the unsupported version; it must respond with the
/// latest version it actually supports.
async fn run_version_negotiation_check(config: &Config) -> CheckResult {
    const NAME: &str = "version_negotiation";
    let agent = match AcpAgent::from_str(&config.agent_command) {
        Ok(agent) => agent,
        Err(e) => return CheckResult::fail(NAME, format!("failed to spawn agent: {e}")),
    };

    let bogus_version = ProtocolVersion::from(999u16);
    let outcome = Client
        .builder()
        .name("acp-validator-version-negotiation")
        .connect_with(agent, move |cx: ConnectionTo<Agent>| async move {
            with_timeout(
                config.timeout,
                "initialize",
                cx.send_request(InitializeRequest::new(bogus_version)).block_task(),
            )
            .await
            .map_err(agent_client_protocol::util::internal_error)
        })
        .await;

    match outcome {
        Ok(response) => {
            let negotiated = response.protocol_version.as_u16();
            if negotiated == 999 {
                CheckResult::fail(
                    NAME,
                    "agent echoed back the unsupported protocolVersion=999 verbatim instead of \
                     negotiating down to a version it supports",
                )
            } else {
                CheckResult::pass(
                    NAME,
                    format!(
                        "requested protocolVersion=999 (unsupported); agent correctly \
                         negotiated down to protocolVersion={negotiated}"
                    ),
                )
            }
        }
        Err(e) => CheckResult::fail(
            NAME,
            format!(
                "agent errored on an unsupported protocol version instead of negotiating \
                 down to a supported one: {e}"
            ),
        ),
    }
}

async fn check_additional_directories(
    cx: &ConnectionTo<Agent>,
    init_response: &InitializeResponse,
    cwd: &std::path::Path,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "session_new_additional_directories";
    if init_response
        .agent_capabilities
        .session_capabilities
        .additional_directories
        .is_none()
    {
        return CheckResult::skip(
            NAME,
            "agent did not advertise session_capabilities.additionalDirectories",
        );
    }

    let extra_dir = std::env::temp_dir();
    match with_timeout(
        timeout,
        "session/new (additionalDirectories)",
        cx.send_request(
            NewSessionRequest::new(cwd.to_path_buf()).additional_directories(vec![extra_dir.clone()]),
        )
        .block_task(),
    )
    .await
    {
        Ok(response) => CheckResult::pass(
            NAME,
            format!(
                "session {} created with additionalDirectories=[{}]",
                response.session_id.0,
                extra_dir.display()
            ),
        ),
        Err(e) => CheckResult::fail(NAME, e),
    }
}

fn text_prompt(text: impl Into<String>) -> Vec<ContentBlock> {
    vec![ContentBlock::Text(TextContent::new(text.into()))]
}

/// Check 5: at least one `session/update` must arrive before the
/// `session/prompt` response resolves, and the response's `stopReason` must
/// be a value the typed schema recognizes (validated implicitly: if the
/// agent sent something outside the `StopReason` enum, deserialization of
/// the response itself would fail and surface as an error here).
async fn check_prompt_streaming(
    cx: &ConnectionTo<Agent>,
    shared: &Shared,
    session_id: &SessionId,
    trigger: &str,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "prompt_streaming";
    shared.drain_updates();

    let result = with_timeout(
        timeout,
        "session/prompt",
        cx.send_request(PromptRequest::new(session_id.clone(), text_prompt(trigger)))
            .block_task(),
    )
    .await;

    let updates = shared.drain_updates();
    match result {
        Ok(response) => {
            if updates.is_empty() {
                CheckResult::fail(
                    NAME,
                    format!(
                        "no session/update notifications arrived before the prompt response \
                         resolved (stopReason={:?})",
                        response.stop_reason
                    ),
                )
            } else {
                let wrong_session = updates
                    .iter()
                    .filter(|(_, n)| n.session_id != *session_id)
                    .count();
                if wrong_session > 0 {
                    CheckResult::fail(
                        NAME,
                        format!(
                            "{wrong_session} of {} session/update notifications referenced a \
                             different sessionId than the one being prompted",
                            updates.len()
                        ),
                    )
                } else {
                    CheckResult::pass(
                        NAME,
                        format!(
                            "{} session/update notification(s) arrived before the response; \
                             stopReason={:?}",
                            updates.len(),
                            response.stop_reason
                        ),
                    )
                }
            }
        }
        Err(e) => CheckResult::fail(NAME, e),
    }
}

/// Check 11: rerun a streaming prompt and additionally assert that no
/// `session/update` for this session arrives *after* the `session/prompt`
/// response has already resolved (the spec requires the Agent to flush all
/// updates before responding, except in the cancellation flow).
async fn check_update_ordering(
    cx: &ConnectionTo<Agent>,
    shared: &Shared,
    session_id: &SessionId,
    trigger: &str,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "update_ordering";
    shared.drain_updates();

    let result = with_timeout(
        timeout,
        "session/prompt",
        cx.send_request(PromptRequest::new(session_id.clone(), text_prompt(trigger)))
            .block_task(),
    )
    .await;

    let response = match result {
        Ok(response) => response,
        Err(e) => return CheckResult::fail(NAME, e),
    };

    // Give any straggler notifications a brief window to arrive so we don't
    // race the transport.
    tokio::time::sleep(Duration::from_millis(200)).await;
    let during = shared.drain_updates();

    if during.is_empty() {
        return CheckResult::skip(
            NAME,
            "agent did not stream any session/update notifications for this trigger",
        );
    }

    let session_updates_for_us: Vec<&(u64, SessionNotification)> = during
        .iter()
        .filter(|(_, n)| n.session_id == *session_id)
        .collect();

    let seqs: Vec<u64> = session_updates_for_us.iter().map(|(seq, _)| *seq).collect();
    let sorted = {
        let mut s = seqs.clone();
        s.sort_unstable();
        s
    };
    if seqs != sorted {
        return CheckResult::fail(
            NAME,
            "session/update notifications for this session were observed out of receipt order",
        );
    }

    CheckResult::pass(
        NAME,
        format!(
            "{} session/update notification(s) observed in coherent receipt order, all \
             delivered before the session/prompt response (stopReason={:?}) with no stragglers \
             arriving afterward",
            session_updates_for_us.len(),
            response.stop_reason
        ),
    )
}

/// Check 6: exercise both permission-response variants: selecting the first
/// offered option, and responding with `Cancelled` (without an accompanying
/// `session/cancel`) to make sure the Agent handles a declined/cancelled
/// permission gracefully rather than hanging.
///
/// The verdict is based on the permission round-trip itself (was it seen,
/// and did the request shape look right) rather than on the prompt turn's
/// final outcome: an agent may legitimately do other things later in the
/// same turn (e.g. exercise unrelated capabilities we didn't advertise) that
/// fail for reasons that have nothing to do with permission handling. A
/// timeout, however, always fails the check: it means the agent never came
/// back to us at all, which permission handling could plausibly be blocking.
async fn check_permission_roundtrip(
    cx: &ConnectionTo<Agent>,
    shared: &Shared,
    session_id: &SessionId,
    trigger: &str,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "permission_roundtrip";

    // Sub-case A: select the first option.
    *shared.permission_mode.lock().unwrap() = PermissionMode::AutoSelectFirst;
    shared.permission_seen.store(false, Ordering::SeqCst);
    let select_prompt = cx
        .send_request(PromptRequest::new(session_id.clone(), text_prompt(trigger)))
        .block_task();
    let select_outcome = tokio::time::timeout(timeout, select_prompt).await;
    let select_seen = shared.permission_seen.swap(false, Ordering::SeqCst);
    let select_shape = shared.permission_last.lock().unwrap().clone();

    if !select_seen {
        return if select_outcome.is_err() {
            CheckResult::fail(
                NAME,
                format!(
                    "agent never sent session/request_permission and the prompt (trigger \
                     {trigger:?}) timed out after {timeout:?}"
                ),
            )
        } else {
            CheckResult::skip(
                NAME,
                format!(
                    "agent never sent session/request_permission for trigger {trigger:?}; \
                     nothing to validate (try --permission-trigger)"
                ),
            )
        };
    }

    let mut notes = Vec::new();
    if let Some(request) = &select_shape {
        if request.session_id != *session_id {
            notes.push(
                "request_permission.sessionId did not match the prompted session".to_string(),
            );
        }
        if request.options.is_empty() {
            notes.push("request_permission.options was empty".to_string());
        }
    }
    match select_outcome {
        Ok(Ok(response)) => notes.push(format!(
            "select-first: prompt completed with stopReason={:?}",
            response.stop_reason
        )),
        Ok(Err(e)) => notes.push(format!(
            "select-first: permission request handled correctly, but the turn later errored \
             for an unrelated reason: {e}"
        )),
        Err(_) => {
            return CheckResult::fail(
                NAME,
                format!(
                    "agent answered session/request_permission but the prompt then hung, \
                     timing out after {timeout:?}"
                ),
            );
        }
    }
    let shape_ok = notes
        .iter()
        .all(|n| !n.contains("did not match") && !n.contains("was empty"));

    // Sub-case B: respond with Cancelled (no session/cancel involved).
    *shared.permission_mode.lock().unwrap() = PermissionMode::AutoCancelled;
    shared.permission_seen.store(false, Ordering::SeqCst);
    let cancelled_prompt = cx
        .send_request(PromptRequest::new(session_id.clone(), text_prompt(trigger)))
        .block_task();
    let cancelled_outcome = tokio::time::timeout(timeout, cancelled_prompt).await;
    let cancelled_seen = shared.permission_seen.swap(false, Ordering::SeqCst);
    *shared.permission_mode.lock().unwrap() = PermissionMode::AutoSelectFirst;

    if !cancelled_seen {
        notes.push(
            "second run (Cancelled outcome) never triggered a permission request; only the \
             select-first variant was validated"
                .to_string(),
        );
        return if shape_ok {
            CheckResult::pass(NAME, notes.join("; "))
        } else {
            CheckResult::fail(NAME, notes.join("; "))
        };
    }

    match cancelled_outcome {
        Ok(Ok(response)) => notes.push(format!(
            "Cancelled-outcome: prompt completed with stopReason={:?}",
            response.stop_reason
        )),
        Ok(Err(e)) => notes.push(format!(
            "Cancelled-outcome: agent handled the Cancelled outcome without hanging, but the \
             turn later errored for an unrelated reason: {e}"
        )),
        Err(_) => {
            return CheckResult::fail(
                NAME,
                format!(
                    "{}; but the agent hung after being told the permission request was \
                     Cancelled (timed out after {timeout:?})",
                    notes.join("; ")
                ),
            );
        }
    }

    if shape_ok {
        CheckResult::pass(NAME, notes.join("; "))
    } else {
        CheckResult::fail(NAME, notes.join("; "))
    }
}

/// Check 7: declare fs read/write client capability (done once at
/// `initialize`) and, during a trigger prompt, serve any `fs/read_text_file`
/// / `fs/write_text_file` calls from an in-memory map, asserting the
/// request shapes (absolute path, matching sessionId).
async fn check_fs_callbacks(
    cx: &ConnectionTo<Agent>,
    shared: &Shared,
    session_id: &SessionId,
    trigger: &str,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "fs_callbacks";
    shared.fs_write_seen.store(false, Ordering::SeqCst);
    shared.fs_read_seen.store(false, Ordering::SeqCst);
    shared.fs_shape_violations.lock().unwrap().clear();

    let prompt = cx
        .send_request(PromptRequest::new(session_id.clone(), text_prompt(trigger)))
        .block_task();
    let outcome = tokio::time::timeout(timeout, prompt).await;

    let wrote = shared.fs_write_seen.load(Ordering::SeqCst);
    let read = shared.fs_read_seen.load(Ordering::SeqCst);
    if !wrote && !read {
        return if outcome.is_err() {
            CheckResult::fail(
                NAME,
                format!(
                    "agent never called an fs/* method and the prompt (trigger {trigger:?}) \
                     timed out after {timeout:?}"
                ),
            )
        } else {
            CheckResult::skip(
                NAME,
                format!(
                    "agent never called fs/read_text_file or fs/write_text_file for trigger \
                     {trigger:?} (try --fs-trigger)"
                ),
            )
        };
    }

    // The fs callback(s) happened, which is what this check validates. What the prompt turn
    // does afterward is out of scope: an agent may legitimately go on to do unrelated things
    // that fail for reasons that have nothing to do with fs handling. A timeout is the
    // exception, since it means the agent never came back to us at all.
    let turn_note = match outcome {
        Ok(Ok(response)) => format!("prompt completed with stopReason={:?}", response.stop_reason),
        Ok(Err(e)) => format!("fs callback(s) handled correctly, but the turn later errored for an unrelated reason: {e}"),
        Err(_) => {
            return CheckResult::fail(
                NAME,
                format!(
                    "fs/write_text_file called={wrote}, fs/read_text_file called={read}, but \
                     the prompt then hung, timing out after {timeout:?}"
                ),
            );
        }
    };

    let violations = shared.fs_shape_violations.lock().unwrap().clone();
    let mut session_mismatches = Vec::new();
    if let Some(req) = shared.fs_last_write.lock().unwrap().as_ref() {
        if req.session_id != *session_id {
            session_mismatches.push("fs/write_text_file.sessionId mismatch".to_string());
        }
    }
    if let Some(req) = shared.fs_last_read.lock().unwrap().as_ref() {
        if req.session_id != *session_id {
            session_mismatches.push("fs/read_text_file.sessionId mismatch".to_string());
        }
    }

    if !violations.is_empty() || !session_mismatches.is_empty() {
        let mut all = violations;
        all.extend(session_mismatches);
        return CheckResult::fail(NAME, all.join("; "));
    }

    CheckResult::pass(
        NAME,
        format!(
            "fs/write_text_file called={wrote}, fs/read_text_file called={read}; request \
             shapes valid; {turn_note}"
        ),
    )
}

/// Check 8: send a (typically multi-step) prompt, wait for it to start
/// producing activity, send `session/cancel`, and — if a permission request
/// happens to be pending — resolve it with the `Cancelled` outcome per the
/// client's spec obligation. Assert the prompt resolves with
/// `stopReason: cancelled`.
async fn check_cancel(
    cx: &ConnectionTo<Agent>,
    shared: &Shared,
    session_id: &SessionId,
    trigger: &str,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "cancel";
    *shared.permission_mode.lock().unwrap() = PermissionMode::Defer;
    *shared.permission_pending.lock().unwrap() = None;
    shared.drain_updates();

    let prompt_future = cx
        .send_request(PromptRequest::new(session_id.clone(), text_prompt(trigger)))
        .block_task();
    tokio::pin!(prompt_future);

    // Give the agent a moment to start processing (either a permission
    // request appears, or at least one session/update is emitted) before we
    // cancel, so we're actually exercising mid-flight cancellation rather
    // than cancelling before anything happened.
    let started = wait_until(timeout.min(Duration::from_secs(3)), || {
        shared.permission_pending.lock().unwrap().is_some() || {
            let updates = shared.updates.lock().unwrap();
            !updates.is_empty()
        }
    })
    .await;

    if let Err(e) = cx.send_notification(CancelNotification::new(session_id.clone())) {
        return CheckResult::fail(NAME, format!("failed to send session/cancel: {e}"));
    }

    let permission_cancelled = if let Some(responder) =
        shared.permission_pending.lock().unwrap().take()
    {
        responder
            .respond(RequestPermissionResponse::new(
                RequestPermissionOutcome::Cancelled,
            ))
            .is_ok()
    } else {
        false
    };

    *shared.permission_mode.lock().unwrap() = PermissionMode::AutoSelectFirst;

    let outcome = tokio::time::timeout(timeout, prompt_future).await;
    match outcome {
        Ok(Ok(response)) => {
            if matches!(response.stop_reason, StopReason::Cancelled) {
                CheckResult::pass(
                    NAME,
                    format!(
                        "session/cancel produced stopReason=cancelled (started_before_cancel={started}, \
                         pending_permission_request_resolved_as_cancelled={permission_cancelled})"
                    ),
                )
            } else {
                CheckResult::fail(
                    NAME,
                    format!(
                        "after session/cancel, prompt resolved with stopReason={:?} instead of \
                         cancelled",
                        response.stop_reason
                    ),
                )
            }
        }
        Ok(Err(e)) => CheckResult::fail(
            NAME,
            format!("prompt returned an error instead of a cancelled stop reason: {e}"),
        ),
        Err(_) => CheckResult::fail(
            NAME,
            format!("prompt did not resolve within {timeout:?} of sending session/cancel"),
        ),
    }
}

/// Check 9: if the session advertised modes, set a different one and assert
/// the request succeeds; a `current_mode_update` notification is a bonus
/// (reported, not required).
async fn check_set_mode(
    cx: &ConnectionTo<Agent>,
    shared: &Shared,
    session_id: &SessionId,
    modes: Option<agent_client_protocol::schema::v1::SessionModeState>,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "set_mode";
    let Some(modes) = modes else {
        return CheckResult::skip(NAME, "session/new response did not include session modes");
    };

    let target = modes
        .available_modes
        .iter()
        .find(|m| m.id != modes.current_mode_id)
        .or_else(|| modes.available_modes.first());
    let Some(target) = target else {
        return CheckResult::skip(NAME, "agent advertised modes but with an empty modes list");
    };
    let target_id = target.id.clone();

    shared.drain_updates();
    let result = with_timeout(
        timeout,
        "session/set_mode",
        cx.send_request(SetSessionModeRequest::new(session_id.clone(), target_id.clone()))
            .block_task(),
    )
    .await;

    if let Err(e) = result {
        return CheckResult::fail(NAME, e);
    }

    tokio::time::sleep(Duration::from_millis(150)).await;
    let saw_notification = shared.drain_updates().into_iter().any(|(_, n)| {
        matches!(
            &n.update,
            SessionUpdate::CurrentModeUpdate(update) if update.current_mode_id == target_id
        )
    });

    CheckResult::pass(
        NAME,
        format!(
            "session/set_mode({target_id}) succeeded; current_mode_update notification observed={saw_notification}"
        ),
    )
}

/// Check 10: if the agent advertised auth methods, authenticate with the
/// first one; if a logout capability is also advertised, log out too.
async fn check_auth(
    cx: &ConnectionTo<Agent>,
    init_response: &InitializeResponse,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "auth";
    let Some(method) = init_response.auth_methods.first() else {
        return CheckResult::skip(NAME, "agent did not advertise any auth methods");
    };
    let method_id = method.id().clone();

    if let Err(e) = with_timeout(
        timeout,
        "authenticate",
        cx.send_request(AuthenticateRequest::new(method_id.clone()))
            .block_task(),
    )
    .await
    {
        return CheckResult::fail(NAME, e);
    }

    if init_response.agent_capabilities.auth.logout.is_none() {
        return CheckResult::pass(
            NAME,
            format!("authenticate({}) succeeded; logout capability not advertised", method_id.0),
        );
    }

    match with_timeout(
        timeout,
        "logout",
        cx.send_request(LogoutRequest::new()).block_task(),
    )
    .await
    {
        Ok(_) => CheckResult::pass(
            NAME,
            format!("authenticate({}) succeeded; logout succeeded", method_id.0),
        ),
        Err(e) => CheckResult::fail(NAME, format!("authenticate succeeded but logout failed: {e}")),
    }
}

/// Check 12: an unrecognized method must be rejected with `-32601` (method
/// not found) rather than crashing the agent or being silently accepted; an
/// unrecognized notification must be ignored without killing the
/// connection, verified by a follow-up prompt.
async fn check_unknown_method(
    cx: &ConnectionTo<Agent>,
    session_id: &SessionId,
    timeout: Duration,
) -> CheckResult {
    const NAME: &str = "unknown_method";

    let request = match UntypedMessage::new("_validator/unknown", serde_json::json!({})) {
        Ok(request) => request,
        Err(e) => return CheckResult::fail(NAME, format!("failed to build request: {e}")),
    };
    let request_outcome =
        tokio::time::timeout(timeout, cx.send_request(request).block_task()).await;

    let request_note = match request_outcome {
        Ok(Ok(_)) => {
            return CheckResult::fail(
                NAME,
                "agent returned a successful result for a made-up method instead of a \
                 method-not-found error",
            );
        }
        Ok(Err(e)) => {
            if matches!(e.code, ErrorCode::MethodNotFound) {
                "unknown request correctly rejected with -32601".to_string()
            } else {
                return CheckResult::fail(
                    NAME,
                    format!(
                        "agent rejected the unknown method but with code {:?} instead of \
                         MethodNotFound (-32601): {}",
                        e.code, e.message
                    ),
                );
            }
        }
        Err(_) => {
            return CheckResult::fail(
                NAME,
                format!("agent did not respond to the unknown method within {timeout:?}"),
            );
        }
    };

    let notification = match UntypedMessage::new("_validator/unknown_notification", serde_json::json!({})) {
        Ok(n) => n,
        Err(e) => return CheckResult::fail(NAME, format!("failed to build notification: {e}")),
    };
    if let Err(e) = cx.send_notification(notification) {
        return CheckResult::fail(NAME, format!("failed to send unknown notification: {e}"));
    }

    // Verify liveness with a normal prompt.
    match with_timeout(
        timeout,
        "session/prompt (liveness check)",
        cx.send_request(PromptRequest::new(session_id.clone(), text_prompt("ping")))
            .block_task(),
    )
    .await
    {
        Ok(_) => CheckResult::pass(
            NAME,
            format!("{request_note}; connection survived an unknown notification (follow-up prompt succeeded)"),
        ),
        Err(e) => CheckResult::fail(
            NAME,
            format!("{request_note}; but the connection did not survive an unknown notification: {e}"),
        ),
    }
}

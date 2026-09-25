"""Shared fixtures for the relay's HTTP apitests.

BLACK-BOX HTTP against a relay this file starts itself: a throwaway SQLite
file, a generated keypair, a free port, and `/healthz` polled until it answers.

WHAT BELONGS HERE. HTTP behaviour: status codes, what a body carries, which
account may see which row. Data shape, store invariants and tenancy at the
query layer belong in the Go tests beside the store — the split bob2's
CLAUDE.md states, so a failure here is always about the wire and never about
SQL.

SET RELAY_BASE_URL to test a relay this harness did not start. The database
reset is then skipped, and every test has to make its own state.
"""
from __future__ import annotations

import base64
import os
import socket
import sqlite3
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path

import pytest
import requests

RELAY_DIR = Path(__file__).resolve().parent.parent
EXTERNAL_BASE_URL = os.environ.get("RELAY_BASE_URL")
READY_TIMEOUT = 30.0
# The operator key the harness sets, so /v1/admin/* is mounted. A constant, not
# a secret: it exists only for the relay this file starts.
OPERATOR_KEY = "apitests-operator-key"

# Kept across a reset: the schema marker, and the event-nid SINGLETON row. The
# row is not deleted and re-created — the relay appends through
# `UPDATE relay_event_nid_seq ... WHERE id = 1`, so removing it turns the next
# event into a 404 ("bump event nid: no rows in result set") rather than into a
# clean start. Its value is reset to zero instead.
KEEP_TABLES = {"relay_meta", "relay_event_nid_seq"}

# The registration accepts published documents, and the harness pins no versions
# (RELAY_TERMS_VERSION and friends are unset), so the relay records whatever a
# client presents. These therefore LABEL an acceptance without naming a legal
# text; pinning them would make every test depend on a copy change.
CONSENT_TERMS = "apitests-terms-v1"
CONSENT_PRIVACY = "apitests-privacy-v1"
# A CONSUMER registration waives the fourteen-day withdrawal right as well — the
# relay refuses to supply the service immediately without it — while a BUSINESS
# registration must NOT send it and neither must an invitation acceptance. See
# the relay's registrationShape for why the three cases differ.
CONSENT_WAIVER = "apitests-withdrawal-v1"


@dataclass(frozen=True)
class RelayStack:
    base_url: str
    # None when the suite was pointed at a relay it does not own.
    db_path: Path | None
    owned: bool


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _build_relay(dest: Path) -> Path:
    """Build the relay once per session. The build cache is the environment's
    business: this passes it through unchanged."""
    binary = dest / "relay"
    proc = subprocess.run(
        ["go", "build", "-o", str(binary), "./cmd/relay"],
        cwd=RELAY_DIR,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"building the relay failed:\n{proc.stdout}\n{proc.stderr}")
    return binary


def _generated_keys() -> tuple[str, str]:
    """A base64 Ed25519 seed for the relay's identity, and a SEPARATE secret for
    credential hashing. They must differ: tokenkey refuses a reused key, which
    is a startup failure and not something to discover in a test."""
    seed = base64.b64encode(os.urandom(32)).decode()
    token_key = base64.b64encode(os.urandom(48)).decode()
    return seed, token_key


def _wait_ready(base_url: str, proc: subprocess.Popen | None = None, log: Path | None = None) -> None:
    """Block until /healthz answers, or say what happened instead. A relay that
    exited is reported with its log rather than waited out, because a missing
    key is the usual reason and 'timed out' hides it."""
    deadline = time.monotonic() + READY_TIMEOUT
    last: object = "no attempt"
    while time.monotonic() < deadline:
        if proc is not None and proc.poll() is not None:
            detail = log.read_text() if log is not None and log.exists() else ""
            raise RuntimeError(
                f"the relay exited with {proc.returncode} before becoming ready:\n{detail}"
            )
        try:
            r = requests.get(f"{base_url}/healthz", timeout=2)
            if r.status_code == 200:
                return
            last = f"HTTP {r.status_code}"
        except requests.RequestException as exc:
            last = exc
        time.sleep(0.05)
    raise RuntimeError(
        f"the relay did not answer /healthz at {base_url} within {READY_TIMEOUT:.0f}s "
        f"(last: {last})"
    )


@pytest.fixture(scope="session")
def relay_stack(tmp_path_factory: pytest.TempPathFactory) -> RelayStack:
    if EXTERNAL_BASE_URL:
        _wait_ready(EXTERNAL_BASE_URL)
        return RelayStack(base_url=EXTERNAL_BASE_URL, db_path=None, owned=False)

    tmp = tmp_path_factory.mktemp("relay")
    binary = _build_relay(tmp)
    db_path = tmp / "relay.db"
    seed, token_key = _generated_keys()
    port = _free_port()

    env = dict(os.environ)
    env.update(
        {
            "PORT": str(port),
            "RELAY_DB_PATH": str(db_path),
            "RELAY_PRIVATE_KEY": seed,
            "RELAY_TOKEN_KEY": token_key,
            "RELAY_OPERATOR_KEY": OPERATOR_KEY,
        }
    )
    # Scrub anything that would change which surfaces mount. An inherited
    # Stripe or SMTP setting would make a test's result depend on the machine.
    for inherited in (
        "RELAY_POSTGRES_URL",
        "RELAY_NATS_URL",
        "RELAY_STRIPE_SECRET_KEY",
        "RELAY_LICENSE_AUTHORITY_KEY",
        "RELAY_LICENSE_AUTHORITY_KEY_FILE",
        "SMTP_HOST",
    ):
        env.pop(inherited, None)

    log_path = tmp / "relay.log"
    log = log_path.open("wb")
    proc = subprocess.Popen([str(binary)], env=env, stdout=log, stderr=subprocess.STDOUT)
    base_url = f"http://127.0.0.1:{port}"
    try:
        _wait_ready(base_url, proc, log_path)
        yield RelayStack(base_url=base_url, db_path=db_path, owned=True)
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait(timeout=5)
        log.close()


def _truncate(db_path: Path) -> None:
    """Empty every relay table.

    The table list is read from the database rather than written here: a list in
    a test is stale the moment a table is added, and silently so.

    Nothing is DROPPED. The relay is running while this executes, and it holds
    prepared statements against the event page tables — dropping one underneath
    it turns the next append into a 500 that appears only when the suite runs as
    a whole, which is exactly how this was found. Emptied page tables plus a
    counter reset to zero are equivalent and safe: the next append reuses page
    zero, which exists and is empty.
    """
    con = sqlite3.connect(db_path)
    try:
        con.execute("PRAGMA foreign_keys=OFF")
        names = [
            row[0]
            for row in con.execute("SELECT name FROM sqlite_master WHERE type='table'")
        ]
        for name in names:
            if name.startswith("sqlite_") or name in KEEP_TABLES:
                continue
            con.execute(f'DELETE FROM "{name}"')
        con.execute("UPDATE relay_event_nid_seq SET last_nid = 0 WHERE id = 1")
        con.commit()
    finally:
        con.close()


@pytest.fixture(autouse=True)
def clean_db(relay_stack: RelayStack) -> object:
    """Start every test from an empty relay, so no test depends on the order it
    ran in. Skipped against a relay this harness does not own: it will not
    truncate a database it did not create."""
    if not relay_stack.owned:
        yield
        return
    _truncate(relay_stack.db_path)
    yield


@pytest.fixture
def base_url(relay_stack: RelayStack) -> str:
    return relay_stack.base_url


def carry_session_cookie(session: requests.Session) -> requests.Session:
    """Make `session` carry the relay's session cookie over plain HTTP.

    The relay marks that cookie Secure UNCONDITIONALLY, with no
    insecure-for-development switch — internal/auth/signin/cookies.go argues the
    case, and the argument is that the flag is how a production deployment ends
    up with session cookies on the wire. A browser would therefore send it only
    over HTTPS, and this harness talks plain HTTP to a process it started on
    loopback.

    So a Set-Cookie is replayed into the jar without the Secure attribute, and a
    clear is honoured. What is under test is whether the cookie AUTHENTICATES a
    request; whether a browser would be willing to transmit it is the browser's
    rule, and testing that would need a certificate this harness has no business
    inventing.
    """

    def adopt(response: requests.Response, **_: object) -> requests.Response:
        for cookie in response.cookies:
            cleared = cookie.value == "" or (
                cookie.expires is not None and cookie.expires < time.time()
            )
            if cleared:
                # The relay only ever sets this one cookie, so a clear is a
                # clear.
                session.cookies.clear()
                continue
            session.cookies.set(cookie.name, cookie.value, path=cookie.path or "/")
        return response

    session.hooks["response"].append(adopt)
    return session


@pytest.fixture
def client() -> requests.Session:
    """A fresh session per test: no cookie survives into the next one, which is
    what makes 'is this person signed in' answerable."""
    s = carry_session_cookie(requests.Session())
    s.headers.update({"Accept": "application/json"})
    return s


# --- shared helpers ---------------------------------------------------------


def signup_body(
    email: str,
    password: str = "correcthorsebattery",
    account_name: str | None = None,
    **extra: object,
) -> dict[str, object]:
    """The body one consumer registration needs, in ONE place.

    It is a contract with the relay: adding a required field should be an edit
    here, not an edit in every test file that registers somebody.
    """
    body: dict[str, object] = {
        "email": email,
        "password": password,
        "consent": {
            "terms_version": CONSENT_TERMS,
            "privacy_version": CONSENT_PRIVACY,
            "withdrawal_waiver_version": CONSENT_WAIVER,
        },
    }
    if account_name is not None:
        body["account_name"] = account_name
    body.update(extra)
    return body


def signup(
    base_url: str,
    email: str,
    password: str = "correcthorsebattery",
    account_name: str | None = None,
) -> requests.Session:
    """Register an account and return the signed-in session it created.

    Every assertion carries `r.text`: a failing status is nearly always the
    relay explaining itself, and without it a failure prints only a number.
    """
    s = carry_session_cookie(requests.Session())
    s.headers.update({"Accept": "application/json"})
    r = s.post(f"{base_url}/v1/auth/signup", json=signup_body(email, password, account_name))
    assert r.status_code == 201, r.text
    return s


def account_id_of(session: requests.Session, base_url: str) -> str:
    """The account the session's user belongs to, from GET /v1/auth/me.

    `/me` returns the session UNWRAPPED — the same object signup puts under
    `session` — so the two do not read alike, which is worth knowing before
    writing an assertion against either.
    """
    r = session.get(f"{base_url}/v1/auth/me")
    assert r.status_code == 200, r.text
    accounts = r.json()["accounts"]
    assert accounts, f"a signed-in user with no account: {r.text}"
    return accounts[0]["id"]


@pytest.fixture
def alice(base_url: str) -> requests.Session:
    """A signed-in owner of one account."""
    return signup(base_url, "alice@acme.test", account_name="Acme")


@pytest.fixture
def bob(base_url: str) -> requests.Session:
    """A signed-in owner of a DIFFERENT account, for tenancy assertions."""
    return signup(base_url, "bob@globex.test", account_name="Globex")

"""Shared fixtures for the VFS apitests.

The suite drives internal/surfaces/vfsapi over HTTP. It needs no running
product: internal/tools/vfsapitest is built and started once per session on a
free port, with SQLite for storage and the acting identity read from the
X-Actor header. Point VFSAPI_BASE_URL at an already-running host to use that
instead.
"""
from __future__ import annotations

import json
import os
import shutil
import socket
import subprocess
import tempfile
import time
from pathlib import Path

import pytest
import requests

# conftest.py sits at <module root>/internal/apitests, and the host is a package
# inside that module.
MODULE_ROOT = Path(__file__).resolve().parent.parent.parent
HOST_PACKAGE = "./internal/tools/vfsapitest"
ACTOR_HEADER = "X-Actor"
PREFIX = "/v1/vfs"


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _wait_ready(origin: str, deadline: float = 30.0) -> None:
    last: Exception | None = None
    end = time.monotonic() + deadline
    while time.monotonic() < end:
        try:
            if requests.get(f"{origin}/ready", timeout=2).status_code == 200:
                return
        except requests.RequestException as exc:  # the host is still coming up
            last = exc
        time.sleep(0.2)
    raise RuntimeError(f"vfsapitest did not become ready at {origin} within {deadline}s (last error: {last})")


@pytest.fixture(scope="session")
def origin() -> str:
    """Scheme and authority of the surface's host, with no path prefix."""
    external = os.environ.get("VFSAPI_BASE_URL", "").strip()
    if external:
        url = external.rstrip("/")
        return url[: -len(PREFIX)] if url.endswith(PREFIX) else url

    port = _free_port()
    workdir = tempfile.mkdtemp(prefix="vfsapitest-")
    supplied = os.environ.get("VFSAPI_TEST_BIN")
    binary = Path(supplied) if supplied else Path(workdir) / "vfsapitest"
    if not supplied:
        build = subprocess.run(
            ["go", "build", "-o", str(binary), HOST_PACKAGE],
            cwd=MODULE_ROOT, capture_output=True, text=True,
        )
        if build.returncode != 0:
            raise RuntimeError(f"building the test host failed:\n{build.stdout}{build.stderr}")
    proc = subprocess.Popen(
        [str(binary), "-addr", f"127.0.0.1:{port}", "-db", str(Path(workdir) / "vfs.db")],
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
    )
    base = f"http://127.0.0.1:{port}"
    try:
        _wait_ready(base)
        yield base
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
        shutil.rmtree(workdir, ignore_errors=True)


@pytest.fixture(scope="session")
def base_url(origin: str) -> str:
    """The surface's own base URL: every test appends the route to this."""
    return f"{origin}{PREFIX}"


@pytest.fixture
def clean_store(origin: str):
    """Empty the store before each test so a test never sees another's rows."""
    response = requests.post(f"{origin}/__reset", timeout=5)
    response.raise_for_status()
    yield


def actor_session(tenant: str, user: str, admin: bool = False) -> requests.Session:
    """A session acting as one identity in one tenant.

    A real deployment resolves this from a session cookie; the host under test
    reads it from the header, which is the whole of the difference.
    """
    session = requests.Session()
    session.headers.update({
        "Accept": "application/json",
        ACTOR_HEADER: json.dumps({"TenantID": tenant, "UserID": user, "Admin": admin}),
    })
    return session


@pytest.fixture
def tenant_a() -> tuple[requests.Session, str]:
    """A session acting as alice, the owner of tenant acme."""
    return actor_session("acme", "alice"), "acme"


@pytest.fixture
def tenant_b() -> tuple[requests.Session, str]:
    """A session acting as ben, the owner of tenant beta."""
    return actor_session("beta", "ben"), "beta"


@pytest.fixture
def member() -> tuple[requests.Session, str]:
    """A session acting as bob: in acme, holding no grants of his own."""
    return actor_session("acme", "bob"), "acme"


@pytest.fixture
def operator() -> requests.Session:
    """A session acting as a platform operator, which names the tenant it works in."""
    return actor_session("", "operator", admin=True)


def operator_params(tenant: str) -> dict[str, str]:
    """The query an operator adds to act in a tenant it does not belong to."""
    return {"tenant_id": tenant}

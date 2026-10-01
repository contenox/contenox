"""HTTP fixtures for a VFS host supplied through VFSAPI_BASE_URL."""
from __future__ import annotations

import json
import os

import pytest
import requests

ACTOR_HEADER = "X-Actor"
PREFIX = "/v1/vfs"


@pytest.fixture(scope="session")
def origin() -> str:
    """Scheme and authority of the surface's host, with no path prefix."""
    external = os.environ.get("VFSAPI_BASE_URL", "").strip()
    if not external:
        pytest.fail("Set VFSAPI_BASE_URL to a running VFS test host; Compose supplies it for api-tests.", pytrace=False)
    url = external.rstrip("/")
    return url[: -len(PREFIX)] if url.endswith(PREFIX) else url


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

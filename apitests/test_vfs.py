"""The operator's file surface, over HTTP, against a relay this harness starts.

One surface, one file: /api/vfs/* is what `contenoxops vfs` drives, so what is
asserted here is the wire the CLI depends on and the authority it must not work
without.
"""
from __future__ import annotations

import json

import pytest
import requests

from conftest import OPERATOR_KEY

TENANT = "platform-blueprints"


@pytest.fixture
def operator() -> requests.Session:
    """A session carrying the operator key, which is the only credential the
    file surface accepts."""
    s = requests.Session()
    s.headers.update(
        {
            "Accept": "application/json",
            "Authorization": f"Bearer {OPERATOR_KEY}",
        }
    )
    return s


def url(base_url: str, path: str, tenant: str | None = TENANT) -> str:
    q = f"?tenant_id={tenant}" if tenant is not None else ""
    return f"{base_url}/api/vfs/{path}{q}"


def upload(
    operator: requests.Session,
    base_url: str,
    name: str,
    content: bytes,
    parent: str = "",
    metadata: dict[str, str] | None = None,
    tenant: str | None = TENANT,
):
    data = {"name": name}
    if parent:
        data["parentid"] = parent
    if metadata is not None:
        data["metadata"] = json.dumps(metadata)
    return operator.post(
        url(base_url, "files", tenant),
        files={"file": (name, content)},
        data=data,
    )


def test_operator_file_surface_round_trip(operator: requests.Session, base_url: str) -> None:
    """Create, list, download and delete, with the metadata a sync keys on."""
    created = upload(operator, base_url, "triage.md", b"# triage\n", metadata={"sha256": "abc"})
    assert created.status_code == 201, created.text
    file_id = created.json()["id"]
    assert created.json()["name"] == "triage.md"

    listed = operator.get(url(base_url, "files"))
    assert listed.status_code == 200, listed.text
    entries = listed.json()
    assert [e["name"] for e in entries] == ["triage.md"], listed.text
    assert entries[0]["metadata"]["sha256"] == "abc", "the digest must survive the round trip"

    downloaded = operator.get(url(base_url, f"files/{file_id}/download"))
    assert downloaded.status_code == 200, downloaded.text
    assert downloaded.content == b"# triage\n"
    # Stored bytes are served as an attachment, never as what an uploader claimed.
    assert downloaded.headers["Content-Type"] == "application/octet-stream"
    assert downloaded.headers["X-Content-Type-Options"] == "nosniff"

    removed = operator.delete(url(base_url, f"files/{file_id}"))
    assert removed.status_code == 200, removed.text
    assert operator.get(url(base_url, "files")).json() == []


def test_folders_nest_and_list_by_path(operator: requests.Session, base_url: str) -> None:
    """A blueprint lives in a folder tree, so a path has to address one."""
    top = operator.post(url(base_url, "folders"), json={"name": "blueprints"})
    assert top.status_code == 201, top.text
    nested = operator.post(
        url(base_url, "folders"),
        json={"name": "invoice", "parentId": top.json()["id"]},
    )
    assert nested.status_code == 201, nested.text

    written = upload(operator, base_url, "triage.md", b"# triage\n", parent=nested.json()["id"])
    assert written.status_code == 201, written.text

    listed = operator.get(url(base_url, "files") + "&path=blueprints/invoice")
    assert listed.status_code == 200, listed.text
    assert [e["name"] for e in listed.json()] == ["triage.md"]
    assert listed.json()[0]["path"] == "blueprints/invoice/triage.md", listed.text


def test_the_surface_is_operator_only(operator: requests.Session, base_url: str) -> None:
    """One bearer reaches every tenant's files, so nothing else may reach it."""
    anonymous = requests.get(url(base_url, "files"))
    assert anonymous.status_code == 401, anonymous.text

    wrong = requests.get(
        url(base_url, "files"), headers={"Authorization": "Bearer not-the-operator-key"}
    )
    assert wrong.status_code == 401, wrong.text

    # A signed-in account is not an operator: the file surface is the platform's.
    from conftest import signup

    alice = signup(base_url, "alice-vfs@acme.test", account_name="Acme")
    assert alice.get(url(base_url, "files")).status_code == 401


def test_an_operator_must_name_a_tenant(operator: requests.Session, base_url: str) -> None:
    """The operator has no tenant of its own, and a default would be a guess."""
    unnamed = operator.get(url(base_url, "files", tenant=None))
    assert unnamed.status_code == 400, unnamed.text


def test_tenants_are_isolated(operator: requests.Session, base_url: str) -> None:
    """Two tenants, one operator key, no leakage between them."""
    assert upload(operator, base_url, "secret.md", b"a", tenant="tenant-a").status_code == 201

    other = operator.get(url(base_url, "files", tenant="tenant-b"))
    assert other.status_code == 200, other.text
    assert other.json() == [], "one tenant's file must not appear in another's listing"

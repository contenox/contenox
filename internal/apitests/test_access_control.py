"""Access-control apitests: /access-control CRUD + /permissions.

Creating or mutating a grant requires manage on the target resource. An
identity gets manage on a file by creating it (the ownership grant), so these
tests upload a file first, then share it.
"""
from __future__ import annotations

import io
import uuid

import requests
import pytest

from conftest import actor_session, operator_params

pytestmark = pytest.mark.usefixtures("clean_store")


def _upload(session: requests.Session, base_url: str, name: str | None = None) -> str:
    name = name or f"f_{uuid.uuid4().hex[:6]}.txt"
    files = {"file": (name, io.BytesIO(b"data"), "text/plain")}
    r = session.post(f"{base_url}/files", files=files, data={"name": name})
    assert r.status_code == 201, r.text
    return r.json()["id"]


# --- permissions ---


def test_permissions_lists_values(tenant_a, base_url):
    owner, _ = tenant_a
    r = owner.get(f"{base_url}/permissions")
    assert r.status_code == 200, r.text
    assert r.json()["permissions"] == ["none", "view", "edit", "manage"]


# --- create gating ---


def test_create_grant_requires_manage_on_resource(tenant_a, base_url):
    owner, _ = tenant_a
    # The owner has no grant on "doc_x" → cannot share it.
    r = owner.post(
        f"{base_url}/access-control",
        json={"identity": "bob", "resource": "doc_x", "resourceType": "files", "permission": "view"},
    )
    assert r.status_code == 403, r.text


def test_owner_grants_member_view_on_owned_file(tenant_a, member, base_url):
    owner, _ = tenant_a
    member_session, _ = member
    file_id = _upload(owner, base_url)

    # The member cannot see it yet.
    assert member_session.get(f"{base_url}/files/{file_id}/download").status_code == 404

    r = owner.post(
        f"{base_url}/access-control",
        json={"identity": "bob", "resource": file_id, "resourceType": "files", "permission": "view"},
    )
    assert r.status_code == 201, r.text

    # Now the member can read it.
    assert member_session.get(f"{base_url}/files/{file_id}/download").status_code == 200


def test_owner_grants_member_edit_but_not_delete(tenant_a, member, base_url):
    """Edit reaches a write and not a destroy: deleting takes manage."""
    owner, _ = tenant_a
    member_session, _ = member
    file_id = _upload(owner, base_url)

    r = owner.post(
        f"{base_url}/access-control",
        json={"identity": "bob", "resource": file_id, "resourceType": "files", "permission": "edit"},
    )
    assert r.status_code == 201, r.text

    files = {"file": ("renamed.txt", io.BytesIO(b"edited"), "text/plain")}
    r = member_session.put(f"{base_url}/files/{file_id}", files=files)
    assert r.status_code == 200, r.text

    r = member_session.delete(f"{base_url}/files/{file_id}")
    assert r.status_code == 403, r.text


def test_create_grant_invalid_permission_400(tenant_a, base_url):
    owner, _ = tenant_a
    file_id = _upload(owner, base_url)
    r = owner.post(
        f"{base_url}/access-control",
        json={"identity": "bob", "resource": file_id, "resourceType": "files", "permission": "banana"},
    )
    assert r.status_code == 400, r.text


def test_access_control_unauthenticated_401(base_url):
    r = requests.get(f"{base_url}/access-control")
    assert r.status_code == 401


# --- CRUD ---


def test_grant_crud(tenant_a, member, base_url):
    owner, _ = tenant_a
    file_id = _upload(owner, base_url)

    r = owner.post(
        f"{base_url}/access-control",
        json={"identity": "bob", "resource": file_id, "resourceType": "files", "permission": "view"},
    )
    assert r.status_code == 201, r.text
    grant_id = r.json()["id"]

    r = owner.get(f"{base_url}/access-control/{grant_id}")
    assert r.status_code == 200
    assert r.json()["permission"] == "view"

    r = owner.put(
        f"{base_url}/access-control/{grant_id}",
        json={"resource": file_id, "resourceType": "files", "permission": "edit"},
    )
    assert r.status_code == 200, r.text
    assert r.json()["permission"] == "edit"

    r = owner.delete(f"{base_url}/access-control/{grant_id}")
    assert r.status_code == 204, r.text

    r = owner.get(f"{base_url}/access-control/{grant_id}")
    assert r.status_code == 404


def test_duplicate_grant_is_409(tenant_a, base_url):
    # Re-granting the same (identity, resource, resourceType) hits the unique
    # constraint. It must surface as 409 Conflict, not a 500.
    owner, _ = tenant_a
    file_id = _upload(owner, base_url)

    grant = {"identity": "bob", "resource": file_id, "resourceType": "files", "permission": "view"}
    r = owner.post(f"{base_url}/access-control", json=grant)
    assert r.status_code == 201, r.text

    r = owner.post(f"{base_url}/access-control", json=grant)
    assert r.status_code == 409, r.text


def test_list_grants_pagination(operator, base_url):
    # Each upload creates an ownership grant, so two uploads give the tenant two
    # grants. limit=1 returns one; the X-Next-Cursor header pages to the other.
    owner = actor_session("acme", "alice")
    _upload(owner, base_url)
    _upload(owner, base_url)

    r = operator.get(f"{base_url}/access-control", params={**operator_params("acme"), "limit": 1})
    assert r.status_code == 200, r.text
    page1 = r.json()["entries"]
    assert len(page1) == 1, page1
    cursor = r.headers.get("X-Next-Cursor")
    assert cursor, "a full page must return a next-page cursor"

    r = operator.get(f"{base_url}/access-control", params={**operator_params("acme"), "limit": 1, "cursor": cursor})
    assert r.status_code == 200, r.text
    page2 = r.json()["entries"]
    assert len(page2) == 1, page2
    assert page1[0]["id"] != page2[0]["id"]   # cursor advanced to the other grant


# --- listing + isolation ---


def test_a_session_lists_only_its_own_grants(tenant_a, member, operator, base_url):
    owner, _ = tenant_a
    member_session, _ = member
    owner_file = _upload(owner, base_url)
    member_file = _upload(member_session, base_url)

    r = member_session.get(f"{base_url}/access-control")
    assert r.status_code == 200, r.text
    entries = r.json()["entries"]
    assert len(entries) == 1, entries
    assert entries[0]["identity"] == "bob"
    assert entries[0]["resource"] == member_file

    # An operator acts across tenants and sees every grant in the one it names.
    r = operator.get(f"{base_url}/access-control", params=operator_params("acme"))
    assert r.status_code == 200, r.text
    resources = {e["resource"] for e in r.json()["entries"]}
    assert {owner_file, member_file} <= resources


def test_grant_tenant_isolation(tenant_a, tenant_b, operator, base_url):
    owner_a, _ = tenant_a
    owner_b, _ = tenant_b
    _upload(owner_a, base_url)  # creates owner_a's ownership grant on the file

    r = owner_a.get(f"{base_url}/access-control")
    assert r.status_code == 200, r.text
    grant_id = r.json()["entries"][0]["id"]

    # Tenant B cannot read tenant A's grant by id.
    assert owner_b.get(f"{base_url}/access-control/{grant_id}").status_code == 404
    # Nor can an operator reach it without naming tenant A.
    assert operator.get(f"{base_url}/access-control/{grant_id}", params=operator_params("beta")).status_code == 404

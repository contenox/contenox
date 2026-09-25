"""VFS apitests: file and folder CRUD over /files + /folders.

Every request goes through a session acting as one identity in one tenant.
Covers multi-tenant isolation and ACL enforcement (revoke ownership → download
blocked).
"""
from __future__ import annotations

import io
import json
import uuid

import pytest
import requests

pytestmark = pytest.mark.usefixtures("clean_store")


# --- shared helpers --------------------------------------------------------


def _upload(session: requests.Session, base_url: str, *, name: str, content: bytes = b"hello vfs", content_type: str = "text/plain", parent_id: str = "") -> dict:
    data: dict[str, str] = {"name": name}
    if parent_id:
        data["parentid"] = parent_id
    files = {"file": (name, io.BytesIO(content), content_type)}
    r = session.post(f"{base_url}/files", files=files, data=data)
    assert r.status_code == 201, f"upload failed: {r.status_code} {r.text}"
    return r.json()


def _create_folder(session: requests.Session, base_url: str, name: str, parent_id: str = "") -> dict:
    payload: dict[str, str] = {"name": name}
    if parent_id:
        payload["parentId"] = parent_id
    r = session.post(f"{base_url}/folders", json=payload)
    assert r.status_code == 201, f"folder create failed: {r.status_code} {r.text}"
    return r.json()


# --- File CRUD -------------------------------------------------------------


def test_create_file(tenant_a, base_url):
    s, _ = tenant_a
    name = f"test_{uuid.uuid4().hex[:6]}.txt"
    file = _upload(s, base_url, name=name, content=b"test content")
    assert file["id"]
    assert file["name"] == name
    assert file["path"] == name
    assert file["contentType"]


def test_get_file_metadata(tenant_a, base_url):
    s, _ = tenant_a
    name = f"meta_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(s, base_url, name=name, content=b"metadata test")
    r = s.get(f"{base_url}/files/{created['id']}")
    assert r.status_code == 200
    meta = r.json()
    assert meta["id"] == created["id"]
    assert meta["name"] == name


def test_file_metadata_round_trips(tenant_a, base_url):
    s, _ = tenant_a
    name = f"kv_{uuid.uuid4().hex[:6]}.md"
    md = {"connector.sourceVersion": "etag-1", "title": "My Doc"}
    files = {"file": (name, io.BytesIO(b"# body"), "text/markdown")}
    r = s.post(
        f"{base_url}/files",
        files=files,
        data={"name": name, "metadata": json.dumps(md)},
    )
    assert r.status_code == 201, r.text
    created = r.json()
    assert created["metadata"] == md

    # Read-back surfaces the same metadata.
    r = s.get(f"{base_url}/files/{created['id']}")
    assert r.status_code == 200, r.text
    assert r.json()["metadata"] == md

    # A listing surfaces it too.
    r = s.get(f"{base_url}/files", params={"path": ""})
    assert r.status_code == 200, r.text
    listed = next(f for f in r.json() if f["id"] == created["id"])
    assert listed["metadata"] == md

    # Update replaces metadata wholesale.
    files = {"file": (name, io.BytesIO(b"# changed"), "text/markdown")}
    r = s.put(
        f"{base_url}/files/{created['id']}",
        files=files,
        data={"metadata": json.dumps({"connector.sourceVersion": "etag-2"})},
    )
    assert r.status_code == 200, r.text
    assert r.json()["metadata"] == {"connector.sourceVersion": "etag-2"}


def test_file_metadata_preserved_on_content_only_update(tenant_a, base_url):
    s, _ = tenant_a
    name = f"keep_{uuid.uuid4().hex[:6]}.md"
    md = {"connector.sourceVersion": "etag-1", "title": "Keep Me"}
    files = {"file": (name, io.BytesIO(b"# body"), "text/markdown")}
    r = s.post(
        f"{base_url}/files",
        files=files,
        data={"name": name, "metadata": json.dumps(md)},
    )
    assert r.status_code == 201, r.text
    fid = r.json()["id"]

    # Content-only update: no "metadata" field ⇒ existing metadata is preserved,
    # not wiped.
    files = {"file": (name, io.BytesIO(b"# new body"), "text/markdown")}
    r = s.put(f"{base_url}/files/{fid}", files=files)
    assert r.status_code == 200, r.text
    assert r.json()["metadata"] == md

    # Explicit empty object clears it.
    files = {"file": (name, io.BytesIO(b"# cleared"), "text/markdown")}
    r = s.put(f"{base_url}/files/{fid}", files=files, data={"metadata": "{}"})
    assert r.status_code == 200, r.text
    assert r.json().get("metadata") in (None, {})


def test_file_metadata_invalid_json_400(tenant_a, base_url):
    s, _ = tenant_a
    name = f"badkv_{uuid.uuid4().hex[:6]}.txt"
    files = {"file": (name, io.BytesIO(b"x"), "text/plain")}
    r = s.post(
        f"{base_url}/files",
        files=files,
        data={"name": name, "metadata": "not-json"},
    )
    assert r.status_code == 400, r.text


def test_file_without_metadata_omits_field(tenant_a, base_url):
    s, _ = tenant_a
    name = f"nokv_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(s, base_url, name=name, content=b"plain")
    assert "metadata" not in created or created["metadata"] in (None, {})


def test_update_file(tenant_a, base_url):
    s, _ = tenant_a
    name = f"update_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(s, base_url, name=name, content=b"original")
    new_content = b"updated content"
    files = {"file": (name, io.BytesIO(new_content), "text/plain")}
    r = s.put(f"{base_url}/files/{created['id']}", files=files)
    assert r.status_code == 200, r.text
    updated = r.json()
    assert updated["size"] == len(new_content)


def test_delete_file(tenant_a, base_url):
    s, _ = tenant_a
    name = f"del_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(s, base_url, name=name, content=b"to delete")
    r = s.delete(f"{base_url}/files/{created['id']}")
    assert r.status_code == 200, r.text
    r2 = s.get(f"{base_url}/files/{created['id']}")
    assert r2.status_code == 404


def test_download_file(tenant_a, base_url):
    s, _ = tenant_a
    name = f"download_{uuid.uuid4().hex[:6]}.bin"
    content = b"binary payload here"
    created = _upload(s, base_url, name=name, content=content, content_type="application/octet-stream")
    r = s.get(f"{base_url}/files/{created['id']}/download")
    assert r.status_code == 200
    assert r.content == content
    assert "Content-Disposition" in r.headers
    r2 = s.get(f"{base_url}/files/{created['id']}/download?skip=true")
    assert r2.status_code == 200
    assert "Content-Disposition" not in r2.headers


def test_rename_file(tenant_a, base_url):
    s, _ = tenant_a
    old_name = f"old_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(s, base_url, name=old_name, content=b"rename me")
    new_name = f"new_{uuid.uuid4().hex[:6]}.txt"
    r = s.put(f"{base_url}/files/{created['id']}/name", json={"name": new_name})
    assert r.status_code == 200, r.text
    result = r.json()
    assert result["name"] == new_name
    assert result["path"] == new_name


def test_move_file_to_folder(tenant_a, base_url):
    s, _ = tenant_a
    folder_name = f"folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(s, base_url, folder_name)
    file_name = f"move_{uuid.uuid4().hex[:6]}.txt"
    file = _upload(s, base_url, name=file_name, content=b"move me")
    assert file["path"] == file_name
    r = s.put(f"{base_url}/files/{file['id']}/move", json={"newParentId": folder["id"]})
    assert r.status_code == 200, r.text
    moved = r.json()
    assert moved["path"] == f"{folder_name}/{file_name}"


def test_move_file_to_root(tenant_a, base_url):
    s, _ = tenant_a
    folder_name = f"src_folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(s, base_url, folder_name)
    file_name = f"toroot_{uuid.uuid4().hex[:6]}.txt"
    file = _upload(s, base_url, name=file_name, content=b"to root", parent_id=folder["id"])
    assert file["path"] == f"{folder_name}/{file_name}"
    r = s.put(f"{base_url}/files/{file['id']}/move", json={"newParentId": ""})
    assert r.status_code == 200, r.text
    moved = r.json()
    assert moved["path"] == file_name


# --- Folder CRUD -----------------------------------------------------------


def test_create_folder(tenant_a, base_url):
    s, _ = tenant_a
    name = f"folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(s, base_url, name)
    assert folder["id"]
    assert folder["name"] == name
    assert folder["path"] == name


def test_create_nested_folder(tenant_a, base_url):
    s, _ = tenant_a
    parent_name = f"parent_{uuid.uuid4().hex[:6]}"
    parent = _create_folder(s, base_url, parent_name)
    child_name = f"child_{uuid.uuid4().hex[:6]}"
    child = _create_folder(s, base_url, child_name, parent_id=parent["id"])
    assert child["parentId"] == parent["id"]
    assert child["path"] == f"{parent_name}/{child_name}"


def test_rename_folder(tenant_a, base_url):
    s, _ = tenant_a
    old_name = f"old_folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(s, base_url, old_name)
    new_name = f"new_folder_{uuid.uuid4().hex[:6]}"
    r = s.put(f"{base_url}/folders/{folder['id']}/name", json={"name": new_name})
    assert r.status_code == 200, r.text
    result = r.json()
    assert result["name"] == new_name
    assert result["path"] == new_name


def test_delete_folder(tenant_a, base_url):
    s, _ = tenant_a
    name = f"del_folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(s, base_url, name)
    r = s.delete(f"{base_url}/folders/{folder['id']}")
    assert r.status_code == 200, r.text


def test_move_folder(tenant_a, base_url):
    s, _ = tenant_a
    parent_name = f"parent_{uuid.uuid4().hex[:6]}"
    parent = _create_folder(s, base_url, parent_name)
    child_name = f"child_{uuid.uuid4().hex[:6]}"
    child = _create_folder(s, base_url, child_name)
    assert child["path"] == child_name
    r = s.put(f"{base_url}/folders/{child['id']}/move", json={"newParentId": parent["id"]})
    assert r.status_code == 200, r.text
    moved = r.json()
    assert moved["path"] == f"{parent_name}/{child_name}"
    assert moved["parentId"] == parent["id"]


# --- Listing ---------------------------------------------------------------


def test_list_root_files(tenant_a, base_url):
    s, _ = tenant_a
    base = uuid.uuid4().hex[:6]
    _upload(s, base_url, name=f"{base}_a.txt", content=b"a")
    _upload(s, base_url, name=f"{base}_b.txt", content=b"b")
    r = s.get(f"{base_url}/files")
    assert r.status_code == 200
    items = r.json()
    assert isinstance(items, list)
    paths = [f["path"] for f in items]
    assert f"{base}_a.txt" in paths
    assert f"{base}_b.txt" in paths


def test_list_files_by_path(tenant_a, base_url):
    s, _ = tenant_a
    folder_name = f"listme_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(s, base_url, folder_name)
    _upload(s, base_url, name="inside_a.txt", content=b"a", parent_id=folder["id"])
    _upload(s, base_url, name="inside_b.txt", content=b"b", parent_id=folder["id"])
    r = s.get(f"{base_url}/files", params={"path": folder_name})
    assert r.status_code == 200
    items = r.json()
    paths = [i["path"] for i in items]
    assert f"{folder_name}/inside_a.txt" in paths
    assert f"{folder_name}/inside_b.txt" in paths


def test_list_files_pagination(tenant_a, base_url):
    # Listing is keyset-paginated: ?limit= bounds the page, the server returns
    # the next-page position in the X-Next-Cursor header, and ?cursor= resumes.
    # Two files in a dedicated folder, limit 1: each page holds one, and the
    # cursor walks to the other.
    s, _ = tenant_a
    folder_name = f"page_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(s, base_url, folder_name)
    _upload(s, base_url, name="f0.txt", content=b"x", parent_id=folder["id"])
    _upload(s, base_url, name="f1.txt", content=b"x", parent_id=folder["id"])

    r = s.get(f"{base_url}/files", params={"path": folder_name, "limit": 1})
    assert r.status_code == 200, r.text
    page1 = r.json()
    assert len(page1) == 1, page1
    cursor = r.headers.get("X-Next-Cursor")
    assert cursor, "a full page must return a next-page cursor"

    r = s.get(f"{base_url}/files", params={"path": folder_name, "limit": 1, "cursor": cursor})
    assert r.status_code == 200, r.text
    page2 = r.json()
    assert len(page2) == 1, page2
    assert page1[0]["path"] != page2[0]["path"]   # cursor advanced to the other file


# --- Error cases -----------------------------------------------------------


def test_get_nonexistent_file_returns_404(tenant_a, base_url):
    s, _ = tenant_a
    r = s.get(f"{base_url}/files/{uuid.uuid4()}")
    assert r.status_code == 404


def test_create_file_missing_file_field_400(tenant_a, base_url):
    s, _ = tenant_a
    r = s.post(f"{base_url}/files", data={"name": "no_file.txt"})
    assert r.status_code == 400


def test_rename_file_missing_name_400(tenant_a, base_url):
    s, _ = tenant_a
    name = f"for_rename_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(s, base_url, name=name, content=b"data")
    r = s.put(f"{base_url}/files/{created['id']}/name", json={"name": ""})
    assert r.status_code == 400


def test_create_folder_missing_name_400(tenant_a, base_url):
    s, _ = tenant_a
    r = s.post(f"{base_url}/folders", json={"name": ""})
    assert r.status_code == 400


def test_unauthenticated_request_is_401(base_url):
    r = requests.get(f"{base_url}/files")
    assert r.status_code == 401


# --- Multi-tenant isolation ------------------------------------------------


def test_tenant_isolation_by_id(tenant_a, tenant_b, base_url):
    """B cannot fetch a file by ID created in A's tenant."""
    sa, _ = tenant_a
    sb, _ = tenant_b
    a_file = _upload(sa, base_url, name="a-secret.txt", content=b"only A can see this")

    r = sb.get(f"{base_url}/files/{a_file['id']}")
    assert r.status_code == 404, "tenant B must not see tenant A's file by ID"

    r = sb.get(f"{base_url}/files/{a_file['id']}/download")
    assert r.status_code == 404, "tenant B must not download tenant A's file"


def test_tenant_isolation_by_path(tenant_a, tenant_b, base_url):
    """A and B can each have a file at the same logical path without colliding."""
    sa, _ = tenant_a
    sb, _ = tenant_b
    a_file = _upload(sa, base_url, name="shared-name.txt", content=b"A")
    b_file = _upload(sb, base_url, name="shared-name.txt", content=b"B")
    assert a_file["id"] != b_file["id"], "tenants must get distinct IDs"

    # Each tenant lists root and sees ONLY their own file with that name.
    r = sa.get(f"{base_url}/files")
    assert r.status_code == 200
    paths_a = [f["path"] for f in r.json()]
    assert paths_a == ["shared-name.txt"]

    r = sb.get(f"{base_url}/files")
    assert r.status_code == 200
    paths_b = [f["path"] for f in r.json()]
    assert paths_b == ["shared-name.txt"]


def test_tenant_b_cannot_delete_a_file(tenant_a, tenant_b, base_url):
    sa, _ = tenant_a
    sb, _ = tenant_b
    a_file = _upload(sa, base_url, name="undeleteable.txt", content=b"safe")
    r = sb.delete(f"{base_url}/files/{a_file['id']}")
    # 404 (tenant filter makes it not exist) or 403 (manage check fails) — either is acceptable
    assert r.status_code in (403, 404), f"expected 403/404, got {r.status_code}: {r.text}"


# --- Ownership + ACL enforcement -------------------------------------------


def test_ownership_grant_inserted_on_create(tenant_a, base_url):
    """Creating a file inserts a manage-level access entry for the creator."""
    s, _ = tenant_a
    created = _upload(s, base_url, name="owned.txt", content=b"mine")
    r = s.get(f"{base_url}/access-control")
    assert r.status_code == 200, r.text
    entries = r.json()["entries"]
    matching = [e for e in entries if e["resource"] == created["id"] and e["resourceType"] == "files"]
    assert len(matching) == 1, f"expected exactly one ownership grant, got {matching}"
    assert matching[0]["permission"] == "manage"


def test_acl_revoke_removes_access(tenant_a, base_url):
    """Revoking the creator's grant on a file blocks every later read of it.

    Proves the ACL check does real work — there is no implicit "you created it"
    bypass. The revoke itself succeeds because the DELETE authorizes against the
    grant's target, where the caller still holds manage.
    """
    s, _ = tenant_a
    created = _upload(s, base_url, name="selfrevoke.txt", content=b"data")
    r = s.get(f"{base_url}/access-control")
    grant_id = next(e["id"] for e in r.json()["entries"] if e["resource"] == created["id"])

    r = s.delete(f"{base_url}/access-control/{grant_id}")
    assert r.status_code == 204, r.text

    # The creator now holds nothing on the file → download reads as absent.
    # Denied is mapped to "not found" so a caller cannot probe ids it may not see.
    r = s.get(f"{base_url}/files/{created['id']}/download")
    assert r.status_code == 404, f"expected 404 after revoke, got {r.status_code}"


def test_delete_file_cleans_up_grants(tenant_a, base_url):
    """When a file is deleted, its access entries go with it."""
    s, _ = tenant_a
    created = _upload(s, base_url, name="cleanup.txt", content=b"x")

    r = s.get(f"{base_url}/access-control")
    assert any(e["resource"] == created["id"] for e in r.json()["entries"]), "the ownership grant should be there first"

    r = s.delete(f"{base_url}/files/{created['id']}")
    assert r.status_code == 200, r.text

    r = s.get(f"{base_url}/access-control")
    assert not any(e["resource"] == created["id"] for e in r.json()["entries"]), "OnDelete should have cleaned grants"


def test_move_folder_into_itself_is_400(tenant_a, base_url):
    s, _ = tenant_a
    folder = _create_folder(s, base_url, "self")
    r = s.put(f"{base_url}/folders/{folder['id']}/move", json={"newParentId": folder["id"]})
    assert r.status_code == 400, r.text


def test_move_folder_into_descendant_is_400(tenant_a, base_url):
    s, _ = tenant_a
    parent = _create_folder(s, base_url, "parent")
    child = _create_folder(s, base_url, "child", parent_id=parent["id"])
    r = s.put(f"{base_url}/folders/{parent['id']}/move", json={"newParentId": child["id"]})
    assert r.status_code == 400, r.text

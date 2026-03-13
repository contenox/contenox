import io
import uuid
import requests
from helpers import assert_status_code


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _upload(base_url, *, name, content=b"hello vfs", content_type="text/plain", parent_id=""):
    """POST /files with multipart/form-data and return the parsed JSON response."""
    data = {"name": name}
    if parent_id:
        data["parentid"] = parent_id
    files = {"file": (name, io.BytesIO(content), content_type)}
    resp = requests.post(f"{base_url}/files", files=files, data=data)
    assert_status_code(resp, 201)
    return resp.json()


def _create_folder(base_url, name, parent_id=""):
    payload = {"name": name}
    if parent_id:
        payload["parentId"] = parent_id
    resp = requests.post(f"{base_url}/folders", json=payload)
    assert_status_code(resp, 201)
    return resp.json()


# ---------------------------------------------------------------------------
# File CRUD
# ---------------------------------------------------------------------------

def test_create_file(base_url):
    name = f"test_{uuid.uuid4().hex[:6]}.txt"
    file = _upload(base_url, name=name, content=b"test content")

    assert file["id"] != ""
    assert file["name"] == name
    assert file["path"] == name  # root file — path == name
    assert file["contentType"] != ""

    # Clean up
    del_resp = requests.delete(f"{base_url}/files/{file['id']}")
    assert_status_code(del_resp, 200)


def test_get_file_metadata(base_url):
    name = f"meta_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(base_url, name=name, content=b"metadata test")

    resp = requests.get(f"{base_url}/files/{created['id']}")
    assert_status_code(resp, 200)
    meta = resp.json()
    assert meta["id"] == created["id"]
    assert meta["name"] == name

    requests.delete(f"{base_url}/files/{created['id']}")


def test_update_file(base_url):
    name = f"update_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(base_url, name=name, content=b"original")

    new_content = b"updated content"
    files = {"file": (name, io.BytesIO(new_content), "text/plain")}
    resp = requests.put(f"{base_url}/files/{created['id']}", files=files)
    assert_status_code(resp, 200)
    updated = resp.json()
    assert updated["size"] == len(new_content)

    requests.delete(f"{base_url}/files/{created['id']}")


def test_delete_file(base_url):
    name = f"del_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(base_url, name=name, content=b"to delete")

    del_resp = requests.delete(f"{base_url}/files/{created['id']}")
    assert_status_code(del_resp, 200)

    get_resp = requests.get(f"{base_url}/files/{created['id']}")
    assert get_resp.status_code in (404, 422, 500), (
        f"Expected not-found after delete, got {get_resp.status_code}"
    )


def test_download_file(base_url):
    name = f"download_{uuid.uuid4().hex[:6]}.bin"
    content = b"binary payload here"
    created = _upload(base_url, name=name, content=content, content_type="application/octet-stream")

    resp = requests.get(f"{base_url}/files/{created['id']}/download")
    assert_status_code(resp, 200)
    assert resp.content == content
    assert "Content-Disposition" in resp.headers

    # skip=true should omit Content-Disposition
    resp2 = requests.get(f"{base_url}/files/{created['id']}/download?skip=true")
    assert_status_code(resp2, 200)
    assert "Content-Disposition" not in resp2.headers

    requests.delete(f"{base_url}/files/{created['id']}")


def test_rename_file(base_url):
    old_name = f"old_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(base_url, name=old_name, content=b"rename me")

    new_name = f"new_{uuid.uuid4().hex[:6]}.txt"
    resp = requests.put(
        f"{base_url}/files/{created['id']}/name",
        json={"name": new_name},
    )
    assert_status_code(resp, 200)
    result = resp.json()
    assert result["name"] == new_name
    assert result["path"] == new_name

    requests.delete(f"{base_url}/files/{created['id']}")


def test_move_file_to_folder(base_url):
    folder_name = f"folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(base_url, folder_name)

    file_name = f"move_{uuid.uuid4().hex[:6]}.txt"
    file = _upload(base_url, name=file_name, content=b"move me")
    assert file["path"] == file_name  # starts at root

    resp = requests.put(
        f"{base_url}/files/{file['id']}/move",
        json={"newParentId": folder["id"]},
    )
    assert_status_code(resp, 200)
    moved = resp.json()
    assert moved["path"] == f"{folder_name}/{file_name}"

    requests.delete(f"{base_url}/files/{moved['id']}")
    requests.delete(f"{base_url}/folders/{folder['id']}")


def test_move_file_to_root(base_url):
    folder_name = f"src_folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(base_url, folder_name)

    file_name = f"toroot_{uuid.uuid4().hex[:6]}.txt"
    file = _upload(base_url, name=file_name, content=b"to root", parent_id=folder["id"])
    assert file["path"] == f"{folder_name}/{file_name}"

    resp = requests.put(
        f"{base_url}/files/{file['id']}/move",
        json={"newParentId": ""},
    )
    assert_status_code(resp, 200)
    moved = resp.json()
    assert moved["path"] == file_name

    requests.delete(f"{base_url}/files/{moved['id']}")
    requests.delete(f"{base_url}/folders/{folder['id']}")


# ---------------------------------------------------------------------------
# Folder CRUD
# ---------------------------------------------------------------------------

def test_create_folder(base_url):
    name = f"folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(base_url, name)

    assert folder["id"] != ""
    assert folder["name"] == name
    assert folder["path"] == name

    requests.delete(f"{base_url}/folders/{folder['id']}")


def test_create_nested_folder(base_url):
    parent_name = f"parent_{uuid.uuid4().hex[:6]}"
    parent = _create_folder(base_url, parent_name)

    child_name = f"child_{uuid.uuid4().hex[:6]}"
    child = _create_folder(base_url, child_name, parent_id=parent["id"])

    assert child["parentId"] == parent["id"]
    assert child["path"] == f"{parent_name}/{child_name}"

    requests.delete(f"{base_url}/folders/{child['id']}")
    requests.delete(f"{base_url}/folders/{parent['id']}")


def test_rename_folder(base_url):
    old_name = f"old_folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(base_url, old_name)

    new_name = f"new_folder_{uuid.uuid4().hex[:6]}"
    resp = requests.put(
        f"{base_url}/folders/{folder['id']}/name",
        json={"name": new_name},
    )
    assert_status_code(resp, 200)
    result = resp.json()
    assert result["name"] == new_name
    assert result["path"] == new_name

    requests.delete(f"{base_url}/folders/{folder['id']}")


def test_delete_folder(base_url):
    name = f"del_folder_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(base_url, name)

    del_resp = requests.delete(f"{base_url}/folders/{folder['id']}")
    assert_status_code(del_resp, 200)


def test_move_folder(base_url):
    parent_name = f"parent_{uuid.uuid4().hex[:6]}"
    parent = _create_folder(base_url, parent_name)

    child_name = f"child_{uuid.uuid4().hex[:6]}"
    child = _create_folder(base_url, child_name)
    assert child["path"] == child_name  # starts at root

    resp = requests.put(
        f"{base_url}/folders/{child['id']}/move",
        json={"newParentId": parent["id"]},
    )
    assert_status_code(resp, 200)
    moved = resp.json()
    assert moved["path"] == f"{parent_name}/{child_name}"
    assert moved["parentId"] == parent["id"]

    requests.delete(f"{base_url}/folders/{moved['id']}")
    requests.delete(f"{base_url}/folders/{parent['id']}")


# ---------------------------------------------------------------------------
# Listing
# ---------------------------------------------------------------------------

def test_list_root_files(base_url):
    base = uuid.uuid4().hex[:6]
    file1 = _upload(base_url, name=f"{base}_a.txt", content=b"a")
    file2 = _upload(base_url, name=f"{base}_b.txt", content=b"b")

    resp = requests.get(f"{base_url}/files")
    assert_status_code(resp, 200)
    all_files = resp.json()
    assert isinstance(all_files, list)
    paths = [f["path"] for f in all_files]
    assert f"{base}_a.txt" in paths
    assert f"{base}_b.txt" in paths

    requests.delete(f"{base_url}/files/{file1['id']}")
    requests.delete(f"{base_url}/files/{file2['id']}")


def test_list_files_by_path(base_url):
    folder_name = f"listme_{uuid.uuid4().hex[:6]}"
    folder = _create_folder(base_url, folder_name)

    f1 = _upload(base_url, name="inside_a.txt", content=b"a", parent_id=folder["id"])
    f2 = _upload(base_url, name="inside_b.txt", content=b"b", parent_id=folder["id"])

    resp = requests.get(f"{base_url}/files?path={folder_name}")
    assert_status_code(resp, 200)
    items = resp.json()
    paths = [i["path"] for i in items]
    assert f"{folder_name}/inside_a.txt" in paths
    assert f"{folder_name}/inside_b.txt" in paths

    requests.delete(f"{base_url}/files/{f1['id']}")
    requests.delete(f"{base_url}/files/{f2['id']}")
    requests.delete(f"{base_url}/folders/{folder['id']}")


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------

def test_get_nonexistent_file_returns_error(base_url):
    resp = requests.get(f"{base_url}/files/{uuid.uuid4()}")
    assert resp.status_code in (404, 422, 500)


def test_create_file_missing_file_field_returns_error(base_url):
    resp = requests.post(f"{base_url}/files", data={"name": "no_file.txt"})
    assert resp.status_code in (400, 422, 500)


def test_rename_file_missing_name_returns_error(base_url):
    name = f"for_rename_{uuid.uuid4().hex[:6]}.txt"
    created = _upload(base_url, name=name, content=b"data")

    resp = requests.put(
        f"{base_url}/files/{created['id']}/name",
        json={"name": ""},
    )
    assert resp.status_code in (400, 422, 500)

    requests.delete(f"{base_url}/files/{created['id']}")


def test_create_folder_missing_name_returns_error(base_url):
    resp = requests.post(f"{base_url}/folders", json={"name": ""})
    assert resp.status_code in (400, 422, 500)

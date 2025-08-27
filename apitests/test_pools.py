import requests
from helpers import assert_status_code

import uuid

def create_test_group(base_url):
    """Helper to create unique group"""
    unique_name = f"TestUnit_GroupAffinity-{uuid.uuid4().hex[:8]}"
    payload = {
        "name": unique_name,
        "purposeType": "testing"
    }
    response = requests.post(f"{base_url}/groups", json=payload)
    assert_status_code(response, 201)
    return response.json()["id"]

def test_create_group(base_url):
    group_id = create_test_group(base_url)
    assert group_id is not None

def test_get_group(base_url):
    group_id = create_test_group(base_url)
    response = requests.get(f"{base_url}/groups/{group_id}")
    assert_status_code(response, 200)
    assert response.json()["id"] == group_id

def test_update_group(base_url):
    group_id = create_test_group(base_url)
    new_name = f"UpdatedGroup-{uuid.uuid4().hex[:8]}"
    update_payload = {"name": new_name}
    response = requests.put(f"{base_url}/groups/{group_id}", json=update_payload)
    assert_status_code(response, 200)
    assert response.json()["name"] == new_name

def test_delete_group(base_url):
    group_id = create_test_group(base_url)
    response = requests.delete(f"{base_url}/groups/{group_id}")
    assert_status_code(response, 200)

    # Verify deletion
    response = requests.get(f"{base_url}/groups/{group_id}")
    assert_status_code(response, 404)

def test_list_groups(base_url):
    initial_response = requests.get(f"{base_url}/groups")
    initial_count = len(initial_response.json())

    group_id = create_test_group(base_url)
    response = requests.get(f"{base_url}/groups")
    assert_status_code(response, 200)
    groups = response.json()
    assert len(groups) == initial_count + 1
    assert any(p["id"] == group_id for p in groups)

def test_assign_backend_to_group(base_url, create_backend_and_assign_to_group):
    data = create_backend_and_assign_to_group
    backend_id = data["backend_id"]
    group_id = data["group_id"]

    # Verify assignment
    response = requests.get(f"{base_url}/backend-affinity/{group_id}/backends")
    assert_status_code(response, 200)
    assert any(b["id"] == backend_id for b in response.json())

def test_group_by_name(base_url):
    """Test retrieving a group by name"""
    # First get all groups to find a known name
    response = requests.get(f"{base_url}/groups")
    assert_status_code(response, 200)

    groups = response.json()
    if groups:
        group_name = groups[0]["name"]

        # Get group by name
        response = requests.get(f"{base_url}/group-by-name/{group_name}")
        assert_status_code(response, 200)

        group = response.json()
        assert group["name"] == group_name

def test_group_by_purpose(base_url):
    """Test filtering groups by purpose"""
    # Get a known purpose type from existing groups
    response = requests.get(f"{base_url}/groups")
    assert_status_code(response, 200)

    groups = response.json()
    if groups:
        purpose_type = groups[0]["purposeType"]

        # Filter by purpose
        response = requests.get(f"{base_url}/group-by-purpose/{purpose_type}")
        assert_status_code(response, 200)

        filtered_groups = response.json()
        assert isinstance(filtered_groups, list)
        assert all(p["purposeType"] == purpose_type for p in filtered_groups)

def test_backend_groups_association(base_url, create_backend_and_assign_to_group):
    """Test getting groups for a backend"""
    backend_id = create_backend_and_assign_to_group["backend_id"]

    response = requests.get(f"{base_url}/backend-affinity/{backend_id}/groups")
    assert_status_code(response, 200)

    groups = response.json()
    assert isinstance(groups, list)

def test_model_groups_association(base_url, create_model_and_assign_to_group):
    """Test getting groups for a model"""
    model_id = create_model_and_assign_to_group["model_id"]

    response = requests.get(f"{base_url}/model-affinity/{model_id}/groups")
    assert_status_code(response, 200)

    groups = response.json()
    assert isinstance(groups, list)

import requests
from helpers import assert_status_code

import uuid

def create_test_pool(base_url):
    """Helper to create unique pool"""
    unique_name = f"TestPool-{uuid.uuid4().hex[:8]}"
    payload = {
        "name": unique_name,
        "purposeType": "testing"
    }
    response = requests.post(f"{base_url}/pools", json=payload)
    assert_status_code(response, 201)
    return response.json()["id"]

def test_create_pool(base_url):
    pool_id = create_test_pool(base_url)
    assert pool_id is not None

def test_get_pool(base_url):
    pool_id = create_test_pool(base_url)
    response = requests.get(f"{base_url}/pools/{pool_id}")
    assert_status_code(response, 200)
    assert response.json()["id"] == pool_id

def test_update_pool(base_url):
    pool_id = create_test_pool(base_url)
    new_name = f"UpdatedPool-{uuid.uuid4().hex[:8]}"
    update_payload = {"name": new_name}
    response = requests.put(f"{base_url}/pools/{pool_id}", json=update_payload)
    assert_status_code(response, 200)
    assert response.json()["name"] == new_name

def test_delete_pool(base_url):
    pool_id = create_test_pool(base_url)
    response = requests.delete(f"{base_url}/pools/{pool_id}")
    assert_status_code(response, 204)

    # Verify deletion
    response = requests.get(f"{base_url}/pools/{pool_id}")
    assert_status_code(response, 404)

def test_list_pools(base_url):
    initial_response = requests.get(f"{base_url}/pools")
    initial_count = len(initial_response.json())

    pool_id = create_test_pool(base_url)
    response = requests.get(f"{base_url}/pools")
    assert_status_code(response, 200)
    pools = response.json()
    assert len(pools) == initial_count + 1
    assert any(p["id"] == pool_id for p in pools)

def test_assign_backend_to_pool(base_url, create_backend_and_assign_to_pool):
    data = create_backend_and_assign_to_pool
    backend_id = data["backend_id"]
    pool_id = data["pool_id"]

    # Verify assignment
    response = requests.get(f"{base_url}/backend-associations/{pool_id}/backends")
    assert_status_code(response, 200)
    assert any(b["id"] == backend_id for b in response.json())

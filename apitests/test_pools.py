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
    assert_status_code(response, 200)

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

def test_pool_by_name(base_url):
    """Test retrieving a pool by name"""
    # First get all pools to find a known name
    response = requests.get(f"{base_url}/pools")
    assert_status_code(response, 200)

    pools = response.json()
    if pools:
        pool_name = pools[0]["name"]

        # Get pool by name
        response = requests.get(f"{base_url}/pool-by-name/{pool_name}")
        assert_status_code(response, 200)

        pool = response.json()
        assert pool["name"] == pool_name

def test_pool_by_purpose(base_url):
    """Test filtering pools by purpose"""
    # Get a known purpose type from existing pools
    response = requests.get(f"{base_url}/pools")
    assert_status_code(response, 200)

    pools = response.json()
    if pools:
        purpose_type = pools[0]["purposeType"]

        # Filter by purpose
        response = requests.get(f"{base_url}/pool-by-purpose/{purpose_type}")
        assert_status_code(response, 200)

        filtered_pools = response.json()
        assert isinstance(filtered_pools, list)
        assert all(p["purposeType"] == purpose_type for p in filtered_pools)

def test_backend_pools_association(base_url, create_backend_and_assign_to_pool):
    """Test getting pools for a backend"""
    backend_id = create_backend_and_assign_to_pool["backend_id"]

    response = requests.get(f"{base_url}/backend-associations/{backend_id}/pools")
    assert_status_code(response, 200)

    pools = response.json()
    assert isinstance(pools, list)

def test_model_pools_association(base_url, create_model_and_assign_to_pool):
    """Test getting pools for a model"""
    model_id = create_model_and_assign_to_pool["model_id"]

    response = requests.get(f"{base_url}/model-associations/{model_id}/pools")
    assert_status_code(response, 200)

    pools = response.json()
    assert isinstance(pools, list)

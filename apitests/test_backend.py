import requests
from helpers import assert_status_code

def test_create_backend(base_url, request):
    ollama_url = "http://bad-test:11434"

    payload = {
        "name": "Test backend",
        "baseUrl": ollama_url,
        "type": "ollama",
    }

    # Step 1: Create backend
    response = requests.post(f"{base_url}/backends", json=payload)
    assert_status_code(response, 201)
    backend = response.json()
    backend_id = backend["id"]
    delete_url = f"{base_url}/backends/{backend_id}"
    del_response = requests.delete(delete_url)
    assert_status_code(del_response, 200)

def test_backend_assigned_to_pool(base_url, create_backend_and_assign_to_pool):
    data = create_backend_and_assign_to_pool
    backend_id = data["backend_id"]
    pool_id = data["pool_id"]

    # Verify assignment by fetching backends under the pool
    list_url = f"{base_url}/backend-associations/{pool_id}/backends"
    response = requests.get(list_url)
    assert_status_code(response, 200)
    backends = response.json()
    assert any(b['id'] == backend_id for b in backends), "Backend not found in pool"

def test_list_backends(base_url):
    response = requests.get(f"{base_url}/backends")
    assert_status_code(response, 200)
    backends = response.json()
    assert isinstance(backends, list)

def test_update_backend(base_url, create_backend_and_assign_to_pool):
    backend_id = create_backend_and_assign_to_pool["backend_id"]
    update_payload = {
        "name": "Updated Backend",
        "baseUrl": "http://new-url:11434",
        "type": "ollama"
    }
    response = requests.put(f"{base_url}/backends/{backend_id}", json=update_payload)
    assert_status_code(response, 200)
    updated = response.json()
    assert updated["name"] == "Updated Backend"
    assert updated["baseUrl"] == "http://new-url:11434"

def test_backend_state_details(base_url, create_backend_and_assign_to_pool):
    backend_id = create_backend_and_assign_to_pool["backend_id"]
    response = requests.get(f"{base_url}/backends/{backend_id}")
    assert_status_code(response, 200)
    backend = response.json()
    assert "models" in backend
    assert "pulledModels" in backend

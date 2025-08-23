import requests
from helpers import assert_status_code
import uuid
import time
from typing import Dict, Any, List

def generate_test_chain(id_suffix: str = "") -> Dict[str, Any]:
    """Generate a test task chain definition with unique ID"""
    if id_suffix is None:
        id_suffix = str(uuid.uuid4())[:8]

    return {
        "id": f"test-chain-{id_suffix}",
        "description": "A test task chain for API testing",
        "tasks": [
            {
                "id": "task1",
                "type": "llm",
                "model": "phi3:3.8b",
                "prompt": "Hello, world!"
            }
        ]
    }

def test_create_task_chain(base_url):
    """Test that a task chain can be created successfully."""
    chain = generate_test_chain()

    response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(response, 201)

    created_chain = response.json()
    assert created_chain["id"] == chain["id"], "ID mismatch in created task chain"
    assert created_chain["description"] == chain["description"], "Description mismatch"
    assert len(created_chain["tasks"]) == 1, "Task count mismatch"
    assert created_chain["tasks"][0]["id"] == "task1", "Task ID mismatch"

    # Clean up
    delete_response = requests.delete(f"{base_url}/taskchains/{chain['id']}")
    assert_status_code(delete_response, 200)

def test_get_task_chain(base_url):
    """Test retrieving an existing task chain."""
    # First create a task chain
    chain = generate_test_chain()
    create_response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(create_response, 201)

    # Now get it
    get_response = requests.get(f"{base_url}/taskchains/{chain['id']}")
    assert_status_code(get_response, 200)

    retrieved_chain = get_response.json()
    assert retrieved_chain["id"] == chain["id"], "ID mismatch in retrieved task chain"
    assert retrieved_chain["description"] == chain["description"], "Description mismatch"
    assert len(retrieved_chain["tasks"]) == 1, "Task count mismatch"

    # Clean up
    delete_response = requests.delete(f"{base_url}/taskchains/{chain['id']}")
    assert_status_code(delete_response, 200)

def test_update_task_chain(base_url):
    """Test updating an existing task chain."""
    # Create a task chain first
    chain = generate_test_chain()
    create_response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(create_response, 201)
    created_chain = create_response.json()

    # Modify the chain
    updated_chain = {
        "id": created_chain["id"],
        "description": "Updated description",
        "tasks": [
            {
                "id": "task1",
                "type": "llm",
                "model": "phi3:3.8b",
                "prompt": "Hello, updated world!"
            },
            {
                "id": "task2",
                "type": "hook",
                "url": "http://example.com/hook"
            }
        ]
    }

    # Update it
    update_response = requests.put(
        f"{base_url}/taskchains/{created_chain['id']}",
        json=updated_chain
    )
    assert_status_code(update_response, 200)

    # Verify the update
    actual_updated = update_response.json()
    assert actual_updated["description"] == "Updated description"
    assert len(actual_updated["tasks"]) == 2
    assert actual_updated["tasks"][1]["id"] == "task2"

    # Clean up
    delete_response = requests.delete(f"{base_url}/taskchains/{created_chain['id']}")
    assert_status_code(delete_response, 200)

def test_delete_task_chain(base_url):
    """Test deleting a task chain."""
    # Create a task chain first
    chain = generate_test_chain()
    create_response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(create_response, 201)

    # Delete it
    delete_response = requests.delete(f"{base_url}/taskchains/{chain['id']}")
    assert_status_code(delete_response, 200)

    # Verify it's gone
    get_response = requests.get(f"{base_url}/taskchains/{chain['id']}")
    assert_status_code(get_response, 404)

def test_list_task_chains(base_url):
    """Test listing task chains with pagination."""
    # Create several test chains
    chain_ids = []
    for i in range(5):
        chain = generate_test_chain(f"list-test-{i}")
        response = requests.post(f"{base_url}/taskchains", json=chain)
        assert_status_code(response, 201)
        chain_ids.append(chain["id"])

    # List all chains
    list_response = requests.get(f"{base_url}/taskchains?limit=10")
    assert_status_code(list_response, 200)
    chains = list_response.json()

    # Verify we got at least the chains we created
    assert isinstance(chains, list)
    assert len(chains) >= 5

def test_get_nonexistent_task_chain(base_url):
    """Test getting a task chain that doesn't exist."""
    nonexistent_id = f"nonexistent-chain-{uuid.uuid4()}"
    response = requests.get(f"{base_url}/taskchains/{nonexistent_id}")
    assert_status_code(response, 404)
    error = response.json()
    assert "not found" in error["error"].lower()

def test_delete_nonexistent_task_chain(base_url):
    """Test deleting a task chain that doesn't exist."""
    nonexistent_id = f"nonexistent-chain-{uuid.uuid4()}"
    response = requests.delete(f"{base_url}/taskchains/{nonexistent_id}")
    assert_status_code(response, 404)
    error = response.json()
    assert "not found" in error["error"].lower()

def test_task_chain_full_workflow(base_url):
    """Test a complete workflow with task chain creation, update, and deletion."""
    # 1. Create a task chain
    chain = generate_test_chain()
    create_response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(create_response, 201)
    created_chain = create_response.json()

    # 2. Verify creation
    assert created_chain["id"] == chain["id"]
    assert len(created_chain["tasks"]) == 1

    # 3. Update the task chain
    created_chain["tasks"].append({
        "id": "task2",
        "type": "embed",
        "model": "nomic-embed-text:latest"
    })

    update_response = requests.put(
        f"{base_url}/taskchains/{created_chain['id']}",
        json=created_chain
    )
    assert_status_code(update_response, 200)
    updated_chain = update_response.json()

    # 4. Verify update
    assert len(updated_chain["tasks"]) == 2

    # 5. Verify persistence by fetching again
    get_response = requests.get(f"{base_url}/taskchains/{created_chain['id']}")
    assert_status_code(get_response, 200)
    fetched_chain = get_response.json()
    assert len(fetched_chain["tasks"]) == 2

    # 6. Delete the task chain
    delete_response = requests.delete(f"{base_url}/taskchains/{created_chain['id']}")
    assert_status_code(delete_response, 200)

    # 7. Verify deletion
    final_get_response = requests.get(f"{base_url}/taskchains/{created_chain['id']}")
    assert_status_code(final_get_response, 404)

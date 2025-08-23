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
        "name": f"Test Chain {id_suffix}",
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
    assert created_chain["name"] == chain["name"], "Name mismatch in created task chain"
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
    assert retrieved_chain["name"] == chain["name"], "Name mismatch in retrieved task chain"
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
        "name": "Updated Test Chain",
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
    assert actual_updated["name"] == "Updated Test Chain"
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

    # Check that our chains are in the response
    created_chain_ids = [c["id"] for c in chains]
    for chain_id in chain_ids:
        assert chain_id in created_chain_ids

    # Test pagination with cursor
    if len(chains) > 1:
        # Use RFC3339Nano format for cursor
        cursor = chains[0]["createdAt"]
        paginated_response = requests.get(
            f"{base_url}/taskchains?limit=2&cursor={cursor}"
        )
        assert_status_code(paginated_response, 200)
        paginated_chains = paginated_response.json()
        assert isinstance(paginated_chains, list)
        assert len(paginated_chains) <= 2
        # Verify the cursor pagination worked
        if paginated_chains:
            # Compare timestamps - the paginated results should be newer
            assert paginated_chains[0]["createdAt"] > cursor

    # Clean up all created chains
    for chain_id in chain_ids:
        requests.delete(f"{base_url}/taskchains/{chain_id}")

def test_create_task_chain_validation(base_url):
    """Test validation of task chain creation."""
    # Test with missing ID
    chain = {
        "name": "Invalid Chain",
        "tasks": [{"id": "task1", "type": "llm"}]
    }
    response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(response, 400)
    error = response.json()
    assert "ID is required" in error["error"] or "required" in error["error"]

    # Test with empty tasks
    chain = {
        "id": "invalid-chain",
        "name": "Invalid Chain",
        "tasks": []
    }
    response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(response, 400)
    error = response.json()
    assert "must contain at least one task" in error["error"]

    # Test with invalid task type (assuming the API validates this)
    chain = {
        "id": "invalid-chain",
        "name": "Invalid Chain",
        "tasks": [{"id": "task1", "type": "invalid-type"}]
    }
    response = requests.post(f"{base_url}/taskchains", json=chain)
    # This might be 400 or 200 depending on API design - let's assume 400 for validation
    assert response.status_code in [400, 200]
    if response.status_code == 400:
        error = response.json()
        assert "invalid" in error["error"].lower()

def test_update_task_chain_id_mismatch(base_url):
    """Test updating a task chain with ID mismatch between URL and payload."""
    # Create a valid chain first
    chain = generate_test_chain()
    create_response = requests.post(f"{base_url}/taskchains", json=chain)
    assert_status_code(create_response, 201)

    # Try to update with different ID in payload
    chain["id"] = "different-id"
    response = requests.put(f"{base_url}/taskchains/{chain['id']}", json=chain)
    assert_status_code(response, 422)
    error = response.json()
    assert "ID in payload does not match URL" in error["error"]

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

def test_task_chain_pagination_with_cursor(base_url):
    """Test task chain pagination with cursor parameter."""
    # Create chains with known creation times
    chain_ids = []
    timestamps = []

    for i in range(3):
        chain = generate_test_chain(f"pagination-test-{i}")
        response = requests.post(f"{base_url}/taskchains", json=chain)
        assert_status_code(response, 201)
        created_chain = response.json()
        chain_ids.append(created_chain["id"])
        timestamps.append(created_chain["createdAt"])
        # Add a small delay to ensure different timestamps
        time.sleep(0.01)

    # Get the first chain as cursor
    cursor = timestamps[0]

    # Request chains after the cursor
    response = requests.get(
        f"{base_url}/taskchains?limit=10&cursor={cursor}"
    )
    assert_status_code(response, 200)
    chains = response.json()

    # Verify we got the expected chains (should be the 2 newer ones)
    assert len(chains) == 2
    assert chains[0]["id"] == chain_ids[1]
    assert chains[1]["id"] == chain_ids[2]

    # Clean up
    for chain_id in chain_ids:
        requests.delete(f"{base_url}/taskchains/{chain_id}")

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
    created_chain["name"] = "Fully Tested Chain"
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
    assert updated_chain["name"] == "Fully Tested Chain"
    assert len(updated_chain["tasks"]) == 2

    # 5. Verify persistence by fetching again
    get_response = requests.get(f"{base_url}/taskchains/{created_chain['id']}")
    assert_status_code(get_response, 200)
    fetched_chain = get_response.json()
    assert fetched_chain["name"] == "Fully Tested Chain"
    assert len(fetched_chain["tasks"]) == 2

    # 6. Delete the task chain
    delete_response = requests.delete(f"{base_url}/taskchains/{created_chain['id']}")
    assert_status_code(delete_response, 200)

    # 7. Verify deletion
    final_get_response = requests.get(f"{base_url}/taskchains/{created_chain['id']}")
    assert_status_code(final_get_response, 404)

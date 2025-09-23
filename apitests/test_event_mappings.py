import requests
import uuid
from helpers import assert_status_code
from typing import Dict, Any


def generate_unique_path(prefix: str = "/test/mapping") -> str:
    """Generate a unique mapping path for testing"""
    return f"{prefix}-{uuid.uuid4().hex[:8]}"


def create_test_mapping(**overrides) -> Dict[str, Any]:
    """Helper to create a valid mapping with defaults + overrides"""
    config = {
        "path": generate_unique_path(),
        "eventType": "test.event",
        "eventSource": "test.source",
        "aggregateType": "test.aggregate",
        "aggregateIDField": "id",
        "eventTypeField": "type",
        "eventSourceField": "source",
        "eventIDField": "event_id",
        "version": 1,
        "metadataMapping": {
            "trace_id": "headers.X-Trace-ID",
            "received_at": "metadata.timestamp"
        }
    }
    config.update(overrides)
    return config


def test_create_mapping_success(base_url):
    """Test successful mapping creation"""
    url = f"{base_url}/mappings"
    mapping = create_test_mapping()

    response = requests.post(url, json=mapping)
    assert_status_code(response, 201)

    data = response.json()
    assert data["path"] == mapping["path"]
    assert data["eventType"] == mapping["eventType"]
    assert data["eventSource"] == mapping["eventSource"]
    assert data["aggregateType"] == mapping["aggregateType"]
    assert data["version"] == mapping["version"]
    assert data["metadataMapping"] == mapping["metadataMapping"]


def test_create_mapping_requires_fields(base_url):
    """Test validation failures for missing required fields"""
    url = f"{base_url}/mappings"

    # Test missing path
    mapping = create_test_mapping()
    del mapping["path"]
    response = requests.post(url, json=mapping)
    assert_status_code(response, 422)

    # Test missing eventType
    mapping = create_test_mapping()
    del mapping["eventType"]
    response = requests.post(url, json=mapping)
    assert_status_code(response, 422)

    # Test missing eventSource
    mapping = create_test_mapping()
    del mapping["eventSource"]
    response = requests.post(url, json=mapping)
    assert_status_code(response, 422)

    # Test missing aggregateType
    mapping = create_test_mapping()
    del mapping["aggregateType"]
    response = requests.post(url, json=mapping)
    assert_status_code(response, 422)

    # Test invalid version (<= 0)
    mapping = create_test_mapping(version=0)
    response = requests.post(url, json=mapping)
    assert_status_code(response, 422)


def test_get_mapping_success(base_url):
    """Test retrieving an existing mapping"""
    # Create mapping first
    mapping = create_test_mapping()
    create_response = requests.post(f"{base_url}/mappings", json=mapping)
    assert_status_code(create_response, 201)

    # Get it using query parameter
    path = mapping["path"]
    get_response = requests.get(f"{base_url}/mapping", params={"path": path})
    assert_status_code(get_response, 200)

    retrieved = get_response.json()
    assert retrieved["path"] == mapping["path"]
    assert retrieved["eventType"] == mapping["eventType"]
    assert retrieved["eventSource"] == mapping["eventSource"]
    assert retrieved["aggregateType"] == mapping["aggregateType"]
    assert retrieved["version"] == mapping["version"]
    assert retrieved["metadataMapping"] == mapping["metadataMapping"]


def test_get_mapping_not_found(base_url):
    """Test retrieving a non-existent mapping"""
    nonexistent_path = f"/nonexistent-{uuid.uuid4().hex[:8]}"
    response = requests.get(f"{base_url}/mapping", params={"path": nonexistent_path})
    assert_status_code(response, 404)


def test_update_mapping_success(base_url):
    """Test updating an existing mapping"""
    # Create mapping first
    mapping = create_test_mapping()
    create_response = requests.post(f"{base_url}/mappings", json=mapping)
    assert_status_code(create_response, 201)

    # Update it using query parameter
    updated_mapping = {
        "path": mapping["path"],  # path from query parameter overrides this
        "eventType": "updated.event",
        "eventSource": "updated.source",
        "aggregateType": "updated.aggregate",
        "aggregateIDField": "new_id",
        "version": 2,
        "metadataMapping": {
            "new_key": "new_value"
        }
    }

    path = mapping["path"]
    update_response = requests.put(f"{base_url}/mapping", params={"path": path}, json=updated_mapping)
    assert_status_code(update_response, 200)

    updated = update_response.json()
    assert updated["path"] == mapping["path"]
    assert updated["eventType"] == "updated.event"
    assert updated["eventSource"] == "updated.source"
    assert updated["aggregateType"] == "updated.aggregate"
    assert updated["aggregateIDField"] == "new_id"
    assert updated["version"] == 2
    assert updated["metadataMapping"] == {"new_key": "new_value"}


def test_update_mapping_not_found(base_url):
    """Test updating a non-existent mapping"""
    nonexistent_path = f"/nonexistent-{uuid.uuid4().hex[:8]}"
    mapping = create_test_mapping(path=nonexistent_path)

    response = requests.put(f"{base_url}/mapping", params={"path": nonexistent_path}, json=mapping)
    assert_status_code(response, 404)


def test_delete_mapping_success(base_url):
    """Test deleting an existing mapping"""
    # Create mapping first
    mapping = create_test_mapping()
    create_response = requests.post(f"{base_url}/mappings", json=mapping)
    assert_status_code(create_response, 201)

    # Delete it using query parameter
    path = mapping["path"]
    delete_response = requests.delete(f"{base_url}/mapping", params={"path": path})
    assert_status_code(delete_response, 200)

    # Verify it's gone
    get_response = requests.get(f"{base_url}/mapping", params={"path": path})
    assert_status_code(get_response, 404)


def test_delete_mapping_not_found(base_url):
    """Test deleting a non-existent mapping"""
    nonexistent_path = f"/nonexistent-{uuid.uuid4().hex[:8]}"
    response = requests.delete(f"{base_url}/mapping", params={"path": nonexistent_path})
    assert_status_code(response, 404)


def test_list_mappings(base_url):
    """Test listing all mappings"""
    # Create a few mappings
    paths = []
    for i in range(3):
        mapping = create_test_mapping(path=generate_unique_path(f"/test/list-{i}"))
        response = requests.post(f"{base_url}/mappings", json=mapping)
        assert_status_code(response, 201)
        paths.append(mapping["path"])

    # List all mappings
    response = requests.get(f"{base_url}/mappings")
    assert_status_code(response, 200)

    mappings = response.json()
    assert isinstance(mappings, list)

    # Verify our mappings are in the list
    created_mappings = [m for m in mappings if m["path"] in paths]
    assert len(created_mappings) == 3

    # Verify structure
    for m in created_mappings:
        assert "path" in m
        assert "eventType" in m
        assert "eventSource" in m
        assert "aggregateType" in m
        assert "version" in m
        assert "metadataMapping" in m


def test_create_mapping_duplicate(base_url):
    """Test creating a mapping with duplicate path"""
    mapping = create_test_mapping()

    # Create first time
    response = requests.post(f"{base_url}/mappings", json=mapping)
    assert_status_code(response, 201)

    # Try to create again with same path
    response = requests.post(f"{base_url}/mappings", json=mapping)
    assert_status_code(response, 409)  # Conflict


def test_mapping_full_workflow(base_url):
    """Test complete workflow: create → get → update → get → delete → get"""
    # 1. Create
    mapping = create_test_mapping()
    create_response = requests.post(f"{base_url}/mappings", json=mapping)
    assert_status_code(create_response, 201)
    created = create_response.json()

    # 2. Get and verify using query parameter
    get_response = requests.get(f"{base_url}/mapping", params={"path": created['path']})
    assert_status_code(get_response, 200)
    retrieved = get_response.json()
    assert retrieved["path"] == created["path"]

    # 3. Update using query parameter
    updated_mapping = {
        "path": created["path"],
        "eventType": "workflow.updated",
        "eventSource": "workflow.source",
        "aggregateType": "workflow.aggregate",
        "version": 2,
        "metadataMapping": {"workflow": "test"}
    }
    update_response = requests.put(f"{base_url}/mapping", params={"path": created['path']}, json=updated_mapping)
    assert_status_code(update_response, 200)
    updated = update_response.json()

    # 4. Get again and verify update using query parameter
    get_response = requests.get(f"{base_url}/mapping", params={"path": created['path']})
    assert_status_code(get_response, 200)
    retrieved = get_response.json()
    assert retrieved["eventType"] == "workflow.updated"
    assert retrieved["version"] == 2

    # 5. Delete using query parameter
    delete_response = requests.delete(f"{base_url}/mapping", params={"path": created['path']})
    assert_status_code(delete_response, 200)

    # 6. Verify deletion using query parameter
    get_response = requests.get(f"{base_url}/mapping", params={"path": created['path']})
    assert_status_code(get_response, 404)

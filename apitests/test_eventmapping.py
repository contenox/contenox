import pytest
import requests
import uuid
import json
from datetime import datetime, timezone, timedelta
from helpers import assert_status_code
from helpers import generate_test_event_payload


def test_get_mapping_success(base_url, create_test_mapping):
    """Test getting a specific mapping by path"""
    mapping_path, mapping_config = create_test_mapping

    # Use query parameter instead of path parameter
    response = requests.get(f"{base_url}/mapping", params={"path": mapping_path})
    assert_status_code(response, 200)

    mapping = response.json()
    assert mapping["path"] == mapping_path
    assert mapping["version"] == mapping_config["version"]
    assert mapping["eventType"] == mapping_config["eventType"]
    assert mapping["eventSource"] == mapping_config["eventSource"]


def test_get_mapping_not_found(base_url):
    """Test getting a non-existent mapping"""
    non_existent_path = f"/non/existent/{str(uuid.uuid4())[:8]}"

    response = requests.get(f"{base_url}/mapping", params={"path": non_existent_path})
    assert_status_code(response, 404)


def test_create_mapping_success(base_url):
    """Test creating a new mapping"""
    new_mapping = {
        "path": f"/test/new/mapping/{str(uuid.uuid4())[:8]}",
        "version": 1,
        "eventType": "payment.processed",
        "eventSource": "payment-service",
        "aggregateType": "payment",
        "aggregateIDField": "data.payment_id",
        "metadataMapping": {
            "customer_id": "data.customer_id",
            "amount": "data.amount"
        }
    }

    response = requests.post(f"{base_url}/mappings", json=new_mapping)
    assert_status_code(response, 201)

    # Verify the mapping was created using query parameter
    get_response = requests.get(f"{base_url}/mapping", params={"path": new_mapping['path']})
    assert_status_code(get_response, 200)

    created_mapping = get_response.json()
    assert created_mapping["path"] == new_mapping["path"]
    assert created_mapping["eventType"] == new_mapping["eventType"]


def test_create_mapping_invalid_data(base_url):
    """Test creating a mapping with invalid data"""
    invalid_mapping = {
        "path": "",  # Empty path
        "version": 0,  # Invalid version
        # Missing required fields
    }

    response = requests.post(f"{base_url}/mappings", json=invalid_mapping)
    assert_status_code(response, 422)


def test_update_mapping_success(base_url, create_test_mapping):
    """Test updating an existing mapping"""
    mapping_path, mapping_config = create_test_mapping

    updated_mapping = mapping_config.copy()
    updated_mapping["eventType"] = "user.updated"
    updated_mapping["metadataMapping"]["new_field"] = "data.new_field"

    # Use query parameter for the path
    response = requests.put(f"{base_url}/mapping", params={"path": mapping_path}, json=updated_mapping)
    assert_status_code(response, 200)

    # Verify the mapping was updated
    get_response = requests.get(f"{base_url}/mapping", params={"path": mapping_path})
    assert_status_code(get_response, 200)

    actual_mapping = get_response.json()
    assert actual_mapping["eventType"] == "user.updated"
    assert "new_field" in actual_mapping["metadataMapping"]


def test_delete_mapping_success(base_url, create_test_mapping):
    """Test deleting a mapping"""
    mapping_path, mapping_config = create_test_mapping

    # First, verify the mapping exists
    get_response = requests.get(f"{base_url}/mapping", params={"path": mapping_path})
    assert_status_code(get_response, 200)

    # Delete the mapping using query parameter
    delete_response = requests.delete(f"{base_url}/mapping", params={"path": mapping_path})
    assert_status_code(delete_response, 200)

    # Verify the mapping is gone
    get_response = requests.get(f"{base_url}/mapping", params={"path": mapping_path})
    assert_status_code(get_response, 404)


def test_delete_mapping_not_found(base_url):
    """Test deleting a non-existent mapping"""
    non_existent_path = f"/non/existent/{str(uuid.uuid4())[:8]}"

    response = requests.delete(f"{base_url}/mapping", params={"path": non_existent_path})
    assert_status_code(response, 404)


def test_list_mappings_includes_created(base_url):
    """Test that list mappings includes newly created mappings"""
    # Create a new mapping
    new_mapping = {
        "path": f"/test/list/mapping/{str(uuid.uuid4())[:8]}",
        "version": 1,
        "eventType": "test.list",
        "eventSource": "test-service",
        "aggregateType": "test",
        "aggregateIDField": "data.test_id"
    }

    create_response = requests.post(f"{base_url}/mappings", json=new_mapping)
    assert_status_code(create_response, 201)

    # List all mappings and verify the new one is included
    list_response = requests.get(f"{base_url}/mappings")
    assert_status_code(list_response, 200)

    mappings = list_response.json()
    assert isinstance(mappings, list)

    # Find our mapping in the list
    test_mapping = None
    for mapping in mappings:
        if mapping.get("path") == new_mapping["path"]:
            test_mapping = mapping
            break

    assert test_mapping is not None, f"Test mapping {new_mapping['path']} not found in list"
    assert test_mapping["eventType"] == new_mapping["eventType"]

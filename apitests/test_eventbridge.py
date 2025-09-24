import requests
import uuid
import json
from helpers import assert_status_code
from datetime import datetime, timezone

# Test data helpers
def generate_test_payload():
    """Generate a test payload for event ingestion"""
    return {
        "id": str(uuid.uuid4()),
        "type": "user.created",
        "source": "auth-service",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "data": {
            "user_id": str(uuid.uuid4()),
            "email": "test@example.com",
            "name": "Test User"
        },
        "metadata": {
            "ip": "192.168.1.1",
            "user_agent": "test-client/1.0"
        }
    }

# Tests for Event Bridge endpoints
def test_ingest_event_success(base_url, create_test_mapping):
    mapping_path, mapping_config = create_test_mapping

    # Generate test payload
    payload = generate_test_payload()

    # Ingest event
    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 201)

    event = response.json()

    # Verify event structure
    assert "id" in event
    assert event["event_type"] == payload["type"]
    assert event["event_source"] == payload["source"]
    assert event["aggregate_type"] == "user"
    assert event["aggregate_id"] == payload["data"]["user_id"]
    assert event["version"] == mapping_config["version"]
    assert "created_at" in event

    # Verify metadata was extracted
    assert "metadata" in event
    event.get("metadata")
    # Verify metadata was extracted
    assert "metadata" in event
    metadata = event["metadata"]

    assert metadata["ip_address"] == payload["metadata"]["ip"]
    assert metadata["user_agent"] == payload["metadata"]["user_agent"]

def test_ingest_event_missing_mapping_path(base_url):
    """Test event ingestion with missing mapping path parameter"""
    payload = generate_test_payload()

    # Attempt to ingest without path parameter
    response = requests.post(f"{base_url}/ingest", json=payload)
    assert_status_code(response, 400)

    error = response.json()
    assert "error" in error

def test_ingest_event_different_event_type(base_url, create_test_mapping):
    """Test event ingestion with a different event type than the mapping default"""
    mapping_path, mapping_config = create_test_mapping

    payload = generate_test_payload()
    # Override the type to test field extraction
    payload["type"] = "user.updated"
    response = requests.get(f"{base_url}/mappings")
    assert_status_code(response, 200)

    mappings = response.json()
    print(mappings)
    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 201)


    event = response.json()
    # Verify that the event type was extracted from the field, not the mapping default
    assert event["event_type"] == "user.updated"

def test_sync_mappings_success(base_url):
    """Test the sync mappings endpoint"""
    response = requests.post(f"{base_url}/sync")
    assert_status_code(response, 200)

    result = response.json()
    assert result == "mappings synchronized"

def test_ingest_event_missing_required_fields(base_url, create_test_mapping):
    """Test event ingestion with payload missing required aggregate ID field"""
    mapping_path, mapping_config = create_test_mapping

    # Create payload missing the required data.user_id field
    payload = {
        "id": str(uuid.uuid4()),
        "type": "user.created",
        "source": "auth-service",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "data": {
            "email": "test@example.com",
            "name": "Test User"
            # Missing user_id field which is required by mapping
        },
        "metadata": {
            "ip": "192.168.1.1",
            "user_agent": "test-client/1.0"
        }
    }

    # Attempt to ingest with missing required field
    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 422)

    error = response.json()
    assert "error" in error
    # Should contain error about missing aggregate ID field
    assert "aggregate ID field" in error["error"]["message"] or "not found in payload" in error["error"]["message"]

def test_list_mappings_success(base_url, create_test_mapping):
    """Test the list mappings endpoint returns all configured mappings"""
    # Create a test mapping first
    mapping_path, mapping_config = create_test_mapping

    # List all mappings
    response = requests.get(f"{base_url}/mappings")
    assert_status_code(response, 200)

    mappings = response.json()

    # Should be a list of mappings
    assert isinstance(mappings, list)
    assert len(mappings) > 0

    # Find our test mapping in the list
    test_mapping = None
    for mapping in mappings:
        if mapping.get("path") == mapping_path:
            test_mapping = mapping
            break

    assert test_mapping is not None, f"Test mapping {mapping_path} not found in list"

    # Verify the mapping properties match what we created
    assert test_mapping["version"] == mapping_config["version"]
    assert test_mapping["eventType"] == mapping_config["eventType"]
    assert test_mapping["eventSource"] == mapping_config["eventSource"]
    assert test_mapping["aggregateType"] == mapping_config["aggregateType"]
    assert test_mapping["aggregateIDField"] == mapping_config["aggregateIDField"]
    assert test_mapping["metadataMapping"] == mapping_config["metadataMapping"]

def test_ingest_event_missing_optional_metadata_fields(base_url, create_test_mapping):
    """Test event ingestion with payload missing some optional metadata fields"""
    mapping_path, mapping_config = create_test_mapping

    # Create payload with only one metadata field (missing user_agent)
    payload = {
        "id": str(uuid.uuid4()),
        "type": "user.created",
        "source": "auth-service",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "data": {
            "user_id": str(uuid.uuid4()),
            "email": "test@example.com",
            "name": "Test User"
        },
        "metadata": {
            "ip": "192.168.1.1"
            # Missing user_agent field which is mapped but optional
        }
    }

    # Should succeed even with missing optional metadata field
    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 201)

    event = response.json()

    # Verify event was created successfully
    assert "id" in event
    assert event["aggregate_id"] == payload["data"]["user_id"]

    # Verify metadata contains only the available field
    assert "metadata" in event
    metadata = event["metadata"]
    assert metadata["ip_address"] == payload["metadata"]["ip"]
    # user_agent should not be present since it wasn't in the payload
    assert "user_agent" not in metadata

def test_ingest_event_nested_jsonpath_fields(base_url):
    """Test event ingestion with deeply nested JSONPath field extraction"""
    # Create a mapping with deeply nested fields
    nested_path = f"/test/nested/mapping/{str(uuid.uuid4())[:8]}"
    nested_mapping_config = {
        "path": nested_path,
        "version": 1,
        "eventType": "order.processed",
        "eventSource": "ecommerce-service",
        "eventTypeField": "event.details.type",
        "eventSourceField": "event.source.system",
        "aggregateType": "order",
        "aggregateTypeField": "",
        "aggregateIDField": "event.payload.order.id",
        "metadataMapping": {
            "customer_id": "event.payload.customer.id",
            "region": "event.context.region.code",
            "trace_id": "headers.x-trace-id"
        }
    }

    # Create the nested mapping
    response = requests.post(f"{base_url}/mappings", json=nested_mapping_config)
    assert response.status_code == 201

    # Sync to ensure it's available
    sync_response = requests.post(f"{base_url}/sync")
    assert sync_response.status_code == 200

    # Create payload with deeply nested structure
    payload = {
        "event": {
            "details": {
                "type": "order.shipped"
            },
            "source": {
                "system": "warehouse-service"
            },
            "payload": {
                "order": {
                    "id": "order-12345"
                },
                "customer": {
                    "id": "cust-67890"
                }
            },
            "context": {
                "region": {
                    "code": "us-west-2"
                }
            }
        }
    }

    # Ingest with nested mapping
    response = requests.post(
        f"{base_url}/ingest?path={nested_path}",
        json=payload
    )
    assert_status_code(response, 201)

    event = response.json()

    # Verify nested field extraction worked correctly
    assert event["event_type"] == "order.shipped"  # from event.details.type
    assert event["event_source"] == "warehouse-service"  # from event.source.system
    assert event["aggregate_id"] == "order-12345"  # from event.payload.order.id
    assert event["aggregate_type"] == "order"

    # Verify nested metadata extraction
    assert "metadata" in event
    metadata = event["metadata"]
    assert metadata["customer_id"] == "cust-67890"  # from event.payload.customer.id
    assert metadata["region"] == "us-west-2"  # from event.context.region.code
    # trace_id won't be present since it's from headers, not payload
    assert "trace_id" not in metadata

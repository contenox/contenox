import requests
import uuid
from helpers import assert_status_code
from datetime import datetime, timezone


def generate_test_payload():
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


def test_ingest_event_success(base_url, create_test_mapping):
    mapping_path, mapping_config = create_test_mapping
    payload = generate_test_payload()

    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 201)

    event = response.json()
    assert "id" in event
    assert event["path"] == mapping_path
    assert event["payload"] == payload
    assert "received_at" in event


def test_ingest_event_missing_mapping_path(base_url):
    payload = generate_test_payload()
    response = requests.post(f"{base_url}/ingest", json=payload)
    assert_status_code(response, 400)

    error = response.json()
    assert "error" in error


def test_ingest_event_different_event_type(base_url, create_test_mapping):
    mapping_path, mapping_config = create_test_mapping
    payload = generate_test_payload()
    payload["type"] = "user.updated"

    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 201)

    event = response.json()
    assert "id" in event
    assert event["path"] == mapping_path
    assert event["payload"]["type"] == "user.updated"
    assert "received_at" in event


def test_sync_mappings_success(base_url):
    response = requests.post(f"{base_url}/sync")
    assert_status_code(response, 200)

    result = response.json()
    assert result == "mappings synchronized"


def test_ingest_event_missing_required_fields(base_url, create_test_mapping):
    mapping_path, mapping_config = create_test_mapping

    payload = {
        "id": str(uuid.uuid4()),
        "type": "user.created",
        "source": "auth-service",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "data": {
            "email": "test@example.com",
            "name": "Test User"
        },
        "metadata": {
            "ip": "192.168.1.1",
            "user_agent": "test-client/1.0"
        }
    }

    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 422)

    error = response.json()
    assert "error" in error
    assert "aggregate ID field" in error["error"]["message"] or "not found in payload" in error["error"]["message"]


def test_list_mappings_success(base_url, create_test_mapping):
    mapping_path, mapping_config = create_test_mapping

    response = requests.get(f"{base_url}/mappings")
    assert_status_code(response, 200)

    mappings = response.json()
    assert isinstance(mappings, list)
    assert len(mappings) > 0

    test_mapping = None
    for mapping in mappings:
        if mapping.get("path") == mapping_path:
            test_mapping = mapping
            break

    assert test_mapping is not None, f"Test mapping {mapping_path} not found in list"

    assert test_mapping["version"] == mapping_config["version"]
    assert test_mapping["eventType"] == mapping_config["eventType"]
    assert test_mapping["eventSource"] == mapping_config["eventSource"]
    assert test_mapping["aggregateType"] == mapping_config["aggregateType"]
    assert test_mapping["aggregateIDField"] == mapping_config["aggregateIDField"]
    assert test_mapping["metadataMapping"] == mapping_config["metadataMapping"]


def test_ingest_event_missing_optional_metadata_fields(base_url, create_test_mapping):
    mapping_path, mapping_config = create_test_mapping

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
        }
    }

    response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(response, 201)

    event = response.json()
    assert "id" in event
    assert event["path"] == mapping_path
    assert event["payload"]["data"]["user_id"] == payload["data"]["user_id"]
    assert event["payload"]["metadata"]["ip"] == "192.168.1.1"
    assert "received_at" in event


def test_ingest_event_nested_jsonpath_fields(base_url):
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

    response = requests.post(f"{base_url}/mappings", json=nested_mapping_config)
    assert response.status_code == 201

    sync_response = requests.post(f"{base_url}/sync")
    assert sync_response.status_code == 200

    payload = {
        "event": {
            "details": {"type": "order.shipped"},
            "source": {"system": "warehouse-service"},
            "payload": {
                "order": {"id": "order-12345"},
                "customer": {"id": "cust-67890"}
            },
            "context": {
                "region": {"code": "us-west-2"}
            }
        }
    }

    response = requests.post(
        f"{base_url}/ingest?path={nested_path}",
        json=payload
    )
    assert_status_code(response, 201)

    event = response.json()
    assert "id" in event
    assert event["path"] == nested_path
    assert event["payload"] == payload
    assert "received_at" in event

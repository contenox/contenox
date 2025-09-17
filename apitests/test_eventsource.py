import pytest
import requests
import uuid
import time
from datetime import datetime, timezone, timedelta
from helpers import assert_status_code
import json
import threading

BASE_EVENT = {
    "event_type": "user.created",
    "event_source": "auth-service",
    "aggregate_id": "",
    "aggregate_type": "user",
    "version": 1,
    "data": {"email": "test@example.com"},
    "metadata": {"ip": "127.0.0.1"}
}

def create_event(**overrides):
    """Helper to create a valid event with defaults + overrides"""
    event = BASE_EVENT.copy()
    event["aggregate_id"] = str(uuid.uuid4())
    event.update(overrides)
    return event

@pytest.fixture
def now_utc():
    return datetime.now(timezone.utc)

@pytest.fixture
def valid_time_range(now_utc):
    return {
        "from": (now_utc - timedelta(hours=1)).isoformat(),
        "to": (now_utc + timedelta(hours=1)).isoformat()
    }

def test_append_event_success(base_url, now_utc):
    """Test successful event creation"""
    url = f"{base_url}/events"
    event = create_event()

    response = requests.post(url, json=event)
    assert_status_code(response, 201)

    data = response.json()
    assert "id" in data, "Event should have an ID"
    assert data["event_type"] == event["event_type"]
    assert data["aggregate_id"] == event["aggregate_id"]
    assert data["version"] == event["version"]
    assert "created_at" in data, "Event should have created_at timestamp"

    # Verify we can parse the created_at as ISO format
    created_at = datetime.fromisoformat(data["created_at"].replace("Z", "+00:00"))
    assert abs((created_at - now_utc).total_seconds()) < 60, "created_at should be close to now"

def test_append_event_requires_fields(base_url):
    """Test validation failures for missing required fields"""
    url = f"{base_url}/events"

    # Test missing event_type
    event = create_event()
    del event["event_type"]
    response = requests.post(url, json=event)
    assert_status_code(response, 422)

    # Test missing aggregate_type
    event = create_event()
    del event["aggregate_type"]
    response = requests.post(url, json=event)
    assert_status_code(response, 422)

    # Test missing aggregate_id
    event = create_event()
    del event["aggregate_id"]
    response = requests.post(url, json=event)
    assert_status_code(response, 422)

    # Test invalid version (<= 0)
    event = create_event(version=0)
    response = requests.post(url, json=event)
    assert_status_code(response, 422)

def test_append_event_time_validation(base_url, now_utc):
    """Test event time validation (±10 minutes)"""
    url = f"{base_url}/events"

    # Too old (> 10 minutes ago)
    event = create_event()
    event["created_at"] = (now_utc - timedelta(minutes=11)).isoformat()
    response = requests.post(url, json=event)
    assert_status_code(response, 422)
    error_data = response.json()
    assert "event is too old" in error_data.get("error", {}).get("message", "").lower()

    # Too new (> 10 minutes in future)
    event = create_event()
    event["created_at"] = (now_utc + timedelta(minutes=11)).isoformat()
    response = requests.post(url, json=event)
    assert_status_code(response, 422)
    error_data = response.json()
    assert "event is too new" in error_data.get("error", {}).get("message", "").lower()

def test_get_events_by_aggregate(base_url, now_utc, valid_time_range):
    """Test retrieving events by aggregate"""
    # Create test event
    event = create_event()
    create_response = requests.post(f"{base_url}/events", json=event)
    assert_status_code(create_response, 201)

    # Query by aggregate
    params = {
        "event_type": event["event_type"],
        "aggregate_type": event["aggregate_type"],
        "aggregate_id": event["aggregate_id"],
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10
    }
    response = requests.get(f"{base_url}/events/aggregate", params=params)
    assert_status_code(response, 200)

    events = response.json()
    assert len(events) == 1
    retrieved_event = events[0]
    assert retrieved_event["id"] == create_response.json()["id"]
    assert retrieved_event["event_type"] == event["event_type"]
    assert retrieved_event["aggregate_id"] == event["aggregate_id"]

def test_get_events_by_type(base_url, now_utc, valid_time_range):
    """Test retrieving events by type"""
    # Create test event
    event = create_event()
    create_response = requests.post(f"{base_url}/events", json=event)
    assert_status_code(create_response, 201)

    # Query by type
    params = {
        "event_type": event["event_type"],
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10
    }
    response = requests.get(f"{base_url}/events/type", params=params)
    assert_status_code(response, 200)

    events = response.json()
    assert len(events) >= 1  # Might be more if other tests ran
    # At least one should match our event
    matching_events = [e for e in events if e["event_type"] == event["event_type"]]
    assert len(matching_events) >= 1

def test_get_events_by_source(base_url, now_utc, valid_time_range):
    """Test retrieving events by source"""
    # Create test event
    event = create_event()
    create_response = requests.post(f"{base_url}/events", json=event)
    assert_status_code(create_response, 201)

    # Query by source
    params = {
        "event_type": event["event_type"],
        "event_source": event["event_source"],
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10
    }
    response = requests.get(f"{base_url}/events/source", params=params)
    assert_status_code(response, 200)

    events = response.json()
    assert len(events) >= 1
    # At least one should match our event
    matching_events = [e for e in events if e["event_source"] == event["event_source"]]
    assert len(matching_events) >= 1

def test_get_event_types_in_range(base_url, now_utc, valid_time_range):
    """Test listing distinct event types in time range"""
    # Create test event to ensure we have at least one type
    event = create_event()
    create_response = requests.post(f"{base_url}/events", json=event)
    assert_status_code(create_response, 201)

    params = {
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10
    }
    response = requests.get(f"{base_url}/events/types", params=params)
    assert_status_code(response, 200)

    event_types = response.json()
    assert isinstance(event_types, list)
    assert len(event_types) > 0
    assert event["event_type"] in event_types

def test_delete_events_by_type_in_range(base_url, now_utc, valid_time_range):
    """Test deleting events by type in range"""
    # Create test event
    event = create_event(event_type="test.delete.me")
    create_response = requests.post(f"{base_url}/events", json=event)
    assert_status_code(create_response, 201)
    event_id = create_response.json()["id"]

    # Verify event exists
    params = {
        "event_type": "test.delete.me",
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10
    }
    get_response = requests.get(f"{base_url}/events/type", params=params)
    assert_status_code(get_response, 200)
    events = get_response.json()
    assert len([e for e in events if e["id"] == event_id]) > 0

    # Delete events
    delete_params = {
        "event_type": "test.delete.me",
        "from": valid_time_range["from"],
        "to": valid_time_range["to"]
    }
    delete_response = requests.delete(f"{base_url}/events/type", params=delete_params)
    assert_status_code(delete_response, 200)

    # Verify event is gone
    get_response = requests.get(f"{base_url}/events/type", params=params)
    assert_status_code(get_response, 200)
    events = get_response.json()
    assert len([e for e in events if e["id"] == event_id]) == 0

def test_query_validation_missing_required_fields(base_url, valid_time_range):
    """Test validation for missing required fields in queries"""
    # Test get_events_by_aggregate missing fields
    params = {
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10
    }
    # Missing event_type
    response = requests.get(f"{base_url}/events/aggregate", params={**params, "aggregate_type": "user", "aggregate_id": "123"})
    assert_status_code(response, 422)

    # Missing aggregate_type
    response = requests.get(f"{base_url}/events/aggregate", params={**params, "event_type": "user.created", "aggregate_id": "123"})
    assert_status_code(response, 422)

    # Missing aggregate_id
    response = requests.get(f"{base_url}/events/aggregate", params={**params, "event_type": "user.created", "aggregate_type": "user"})
    assert_status_code(response, 422)

    # Test get_events_by_source missing event_source
    params = {
        "event_type": "user.created",
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10
    }
    response = requests.get(f"{base_url}/events/source", params=params)
    assert_status_code(response, 422)

def test_query_validation_invalid_time_format(base_url):
    """Test validation for invalid time formats"""
    event = create_event()
    create_response = requests.post(f"{base_url}/events", json=event)
    assert_status_code(create_response, 201)

    # Test with invalid time format
    params = {
        "event_type": event["event_type"],
        "aggregate_type": event["aggregate_type"],
        "aggregate_id": event["aggregate_id"],
        "from": "invalid-time",
        "to": (datetime.now(timezone.utc) + timedelta(hours=1)).isoformat(),
        "limit": 10
    }
    response = requests.get(f"{base_url}/events/aggregate", params=params)
    assert_status_code(response, 422)

def test_query_validation_invalid_limit(base_url, valid_time_range):
    """Test validation for invalid limit values"""
    params = {
        "event_type": "user.created",
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": "0"  # Invalid limit
    }
    response = requests.get(f"{base_url}/events/type", params=params)
    assert_status_code(response, 422)

    params["limit"] = "-5"  # Negative limit
    response = requests.get(f"{base_url}/events/type", params=params)
    assert_status_code(response, 422)

def test_delete_validation_missing_fields(base_url, valid_time_range):
    """Test validation for missing required fields in delete operation"""
    # Missing event_type
    params = {
        "from": valid_time_range["from"],
        "to": valid_time_range["to"]
    }
    response = requests.delete(f"{base_url}/events/type", params=params)
    assert_status_code(response, 422)

    # Missing from
    params = {
        "event_type": "user.created",
        "to": valid_time_range["to"]
    }
    response = requests.delete(f"{base_url}/events/type", params=params)
    assert_status_code(response, 422)

    # Missing to
    params = {
        "event_type": "user.created",
        "from": valid_time_range["from"]
    }
    response = requests.delete(f"{base_url}/events/type", params=params)
    assert_status_code(response, 422)

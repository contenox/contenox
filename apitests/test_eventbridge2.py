import pytest
import requests
import uuid
import json
from datetime import datetime, timezone, timedelta
from helpers import assert_status_code
from helpers import generate_test_event_payload


# Add these tests to your event bridge test file

def test_replay_event_success(base_url, create_test_mapping):
    """Test successful event replay by NID"""
    # First, ingest an event to have something to replay
    mapping_path, mapping_config = create_test_mapping
    payload = generate_test_event_payload()

    ingest_response = requests.post(
        f"{base_url}/ingest?path={mapping_path}",
        json=payload
    )
    assert_status_code(ingest_response, 201)

    # Get the raw event to find its NID
    now = datetime.now(timezone.utc)
    from_time = (now - timedelta(minutes=5)).isoformat()
    to_time = (now + timedelta(minutes=5)).isoformat()

    list_response = requests.get(
        f"{base_url}/raw-events",
        params={"from": from_time, "to": to_time, "limit": 10}
    )
    assert_status_code(list_response, 200)

    raw_events = list_response.json()
    assert len(raw_events) > 0

    # Use the first raw event's NID for replay
    nid = raw_events[0]["nid"]

    # Replay the event
    replay_response = requests.post(
        f"{base_url}/replay",
        params={
            "nid": nid,
            "from": from_time,
            "to": to_time
        }
    )
    assert_status_code(replay_response, 202)

    result = replay_response.json()
    assert result == "event replayed"


def test_replay_event_missing_parameters(base_url):
    """Test replay event validation for missing parameters"""
    # Missing nid
    response = requests.post(f"{base_url}/replay")
    assert_status_code(response, 400)

    # Missing from
    response = requests.post(f"{base_url}/replay?nid=123")
    assert_status_code(response, 400)

    # Missing to
    now = datetime.now(timezone.utc)
    from_time = (now - timedelta(minutes=5)).isoformat()
    response = requests.post(
        f"{base_url}/replay",
        params={"nid": "123", "from": from_time}
    )
    assert_status_code(response, 400)


def test_replay_event_invalid_nid(base_url):
    """Test replay event with invalid NID"""
    now = datetime.now(timezone.utc)
    from_time = (now - timedelta(minutes=5)).isoformat()
    to_time = (now + timedelta(minutes=5)).isoformat()

    response = requests.post(
        f"{base_url}/replay",
        params={
            "nid": "invalid_nid",
            "from": from_time,
            "to": to_time
        }
    )
    assert_status_code(response, 422)


def test_replay_event_not_found(base_url):
    """Test replay event with non-existent NID"""
    now = datetime.now(timezone.utc)
    from_time = (now - timedelta(minutes=5)).isoformat()
    to_time = (now + timedelta(minutes=5)).isoformat()

    response = requests.post(
        f"{base_url}/replay",
        params={
            "nid": "999999",  # Non-existent NID
            "from": from_time,
            "to": to_time
        }
    )
    # This might return 404 or 400 depending on implementation
    assert response.status_code in [400, 404]

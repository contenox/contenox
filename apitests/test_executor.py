import requests
import uuid
import datetime
from datetime import datetime, timezone, timedelta
import time
from helpers import assert_status_code, generate_unique_name
from typing import Dict, Any

def test_executor_sync_endpoint(base_url):
    """
    Test that the executor sync endpoint is accessible and returns the expected response.
    This is a basic smoke test for the sync endpoint.
    """
    response = requests.post(f"{base_url}/executor/sync")
    assert_status_code(response, 200)

def test_goja_function_with_builtins(base_url):
    """
    Test a Goja function that uses built-in functions (sendEvent, executeTask, etc.)
    This is a more basic test that doesn't require a mock HTTP server.
    """

    # Generate a valid JavaScript function name (no hyphens)
    base_name = generate_unique_name("test_goja_mutate").replace("-", "_")
    function_name = base_name
    event_type = generate_unique_name("test.goja.mutate")
    mutated_event_type = generate_unique_name("mutated.goja.event")

    # The JavaScript function name MUST be a valid JS identifier
    goja_script = f"""
function {function_name}(event) {{
  var modifiedData = {{
    processedByGoja: true,
    originalEventType: event.eventType,  // Changed from event.event_type to event.eventType
    originalData: event.data,
    testId: event.data.test_id
  }};

  // Send a new, mutated event
  var sendResult = sendEvent("{mutated_event_type}", modifiedData);
  return sendResult;
}}
"""
    now_utc = datetime.now(timezone.utc)

    function_payload = {
        "name": function_name,
        "description": "Test Goja function that mutates and sends an event",
        "scriptType": "goja",
        "script": goja_script
    }
    function_response = requests.post(f"{base_url}/functions", json=function_payload)
    assert_status_code(function_response, 201)

    trigger_name = generate_unique_name("test-mutate-trigger")
    trigger_payload = {
        "name": trigger_name,
        "listenFor": {"type": event_type},
        "type": "function",
        "function": function_name
    }
    trigger_response = requests.post(f"{base_url}/event-triggers", json=trigger_payload)
    assert_status_code(trigger_response, 201)

    sync_response = requests.post(f"{base_url}/executor/sync")
    assert_status_code(sync_response, 200)

    time.sleep(5)

    original_event_id = str(uuid.uuid4())
    test_id = str(uuid.uuid4())
    test_event = {
        "id": original_event_id,
        "event_type": event_type,
        "event_source": "test-source",
        "aggregate_id": original_event_id,
        "aggregate_type": "test",
        "version": 1,
        "data": {"initial_data": "value", "test_id": test_id}
    }
    event_response = requests.post(f"{base_url}/events", json=test_event)
    assert_status_code(event_response, 201)
    time.sleep(5)

    valid_time_range = {
        "from": (now_utc - timedelta(minutes=5)).isoformat(),
        "to": (now_utc + timedelta(minutes=5)).isoformat()
    }

    # First, verify the original event was stored
    params = {
        "from": valid_time_range["from"],
        "to": valid_time_range["to"],
        "limit": 10,
        "event_type": event_type
    }
    verify_response = requests.get(f"{base_url}/events/type", params=params)
    assert_status_code(verify_response, 200)
    events = verify_response.json()
    assert len(events) >= 1
    original_event = next((e for e in events if e.get("data", {}).get("test_id") == test_id), None)
    assert original_event is not None, "Original event not found"
    assert original_event["id"] == original_event_id

    # Now check for the mutated event
    params["event_type"] = mutated_event_type
    get_response = requests.get(f"{base_url}/events/type", params=params)
    assert_status_code(get_response, 200)
    mutated_events = get_response.json()
    assert len(mutated_events) >= 1

    found_event = next((e for e in mutated_events if e.get("data", {}).get("testId") == test_id), None)
    assert found_event is not None, f"Mutated event not found for test_id: {test_id}"

    assert found_event["event_type"] == mutated_event_type
    assert found_event["data"]["processedByGoja"] is True
    assert found_event["data"]["originalEventType"] == event_type
    assert found_event["data"]["originalData"]["initial_data"] == "value"
    assert found_event["data"]["testId"] == test_id

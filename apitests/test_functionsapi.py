import requests
import uuid
import pytest
from helpers import assert_status_code

def test_create_function(base_url):
    """Test that a user can create a function."""
    payload = {
        "name": "test-function-" + str(uuid.uuid4())[:8],
        "description": "A test function",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
    }

    response = requests.post(f"{base_url}/functions", json=payload)
    assert_status_code(response, 201)
    function = response.json()

    assert function["name"] == payload["name"], "Function name does not match"
    assert function["scriptType"] == payload["scriptType"], "Script type does not match"

    # Clean up
    delete_url = f"{base_url}/functions/{function['name']}"
    del_response = requests.delete(delete_url)
    assert_status_code(del_response, 200)

def test_list_functions(base_url):
    """Test listing all functions."""
    response = requests.get(f"{base_url}/functions")
    assert_status_code(response, 200)

    functions = response.json()
    assert isinstance(functions, list), "Expected list of functions"

def test_get_function(base_url):
    """Test getting a specific function."""
    # First create a function
    function_name = "test-get-function-" + str(uuid.uuid4())[:8]
    payload = {
        "name": function_name,
        "description": "A test function for get",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
        "actionType": "chain",
        "actionTarget": "test-chain"
    }

    create_response = requests.post(f"{base_url}/functions", json=payload)
    assert_status_code(create_response, 201)

    # Now get it
    get_response = requests.get(f"{base_url}/functions/{function_name}")
    assert_status_code(get_response, 200)

    function = get_response.json()
    assert function["name"] == function_name, "Function name does not match"

    # Clean up
    delete_url = f"{base_url}/functions/{function_name}"
    del_response = requests.delete(delete_url)
    assert_status_code(del_response, 200)

def test_update_function(base_url):
    """Test updating a function."""
    # First create a function
    function_name = "test-update-function-" + str(uuid.uuid4())[:8]
    payload = {
        "name": function_name,
        "description": "A test function for update",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
        "actionType": "chain",
        "actionTarget": "test-chain"
    }

    create_response = requests.post(f"{base_url}/functions", json=payload)
    assert_status_code(create_response, 201)

    # Update the function
    update_payload = {
        "name": function_name,
        "description": "Updated description",
        "scriptType": "goja",
        "script": "function handler(input) { return input.toUpperCase(); }",
    }

    update_response = requests.put(f"{base_url}/functions/{function_name}", json=update_payload)
    assert_status_code(update_response, 200)

    updated_function = update_response.json()
    assert updated_function["description"] == "Updated description", "Description was not updated"

    # Clean up
    delete_url = f"{base_url}/functions/{function_name}"
    del_response = requests.delete(delete_url)
    assert_status_code(del_response, 200)

def test_delete_function(base_url):
    """Test deleting a function."""
    # First create a function
    function_name = "test-delete-function-" + str(uuid.uuid4())[:8]
    payload = {
        "name": function_name,
        "description": "A test function for deletion",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
    }

    create_response = requests.post(f"{base_url}/functions", json=payload)
    assert_status_code(create_response, 201)

    # Verify it exists
    get_response = requests.get(f"{base_url}/functions/{function_name}")
    assert_status_code(get_response, 200)

    # Delete it
    delete_response = requests.delete(f"{base_url}/functions/{function_name}")
    assert_status_code(delete_response, 200)

    # Verify it's gone
    get_response = requests.get(f"{base_url}/functions/{function_name}")
    assert get_response.status_code == 404, "Function should not exist after deletion"

def test_create_event_trigger(base_url):
    """Test that a user can create an event trigger."""
    # First create a function to reference
    function_name = "test-function-" + str(uuid.uuid4())[:8]
    function_payload = {
        "name": function_name,
        "description": "A test function",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
        "actionType": "chain",
        "actionTarget": "test-chain"
    }

    function_response = requests.post(f"{base_url}/functions", json=function_payload)
    assert_status_code(function_response, 201)

    # Now create the event trigger
    payload = {
        "name": "test-trigger-" + str(uuid.uuid4())[:8],
        "description": "A test event trigger",
        "listenFor": {"type": "test.event.type"},
        "type": "function",
        "function": function_name
    }

    response = requests.post(f"{base_url}/event-triggers", json=payload)
    assert_status_code(response, 201)
    trigger = response.json()

    assert trigger["name"] == payload["name"], "Trigger name does not match"
    assert trigger["function"] == function_name, "Function reference does not match"

    # Clean up
    delete_trigger_url = f"{base_url}/event-triggers/{trigger['name']}"
    del_trigger_response = requests.delete(delete_trigger_url)
    assert_status_code(del_trigger_response, 200)

    delete_function_url = f"{base_url}/functions/{function_name}"
    del_function_response = requests.delete(delete_function_url)
    assert_status_code(del_function_response, 200)

def test_list_event_triggers(base_url):
    """Test listing all event triggers."""
    response = requests.get(f"{base_url}/event-triggers")
    assert_status_code(response, 200)

    triggers = response.json()
    assert isinstance(triggers, list), "Expected list of triggers"

def test_get_event_trigger(base_url):
    """Test getting a specific event trigger."""
    # First create a function to reference
    function_name = "test-function-" + str(uuid.uuid4())[:8]
    function_payload = {
        "name": function_name,
        "description": "A test function",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
        "actionType": "chain",
        "actionTarget": "test-chain"
    }

    function_response = requests.post(f"{base_url}/functions", json=function_payload)
    assert_status_code(function_response, 201)

    # Create the event trigger
    trigger_name = "test-get-trigger-" + str(uuid.uuid4())[:8]
    payload = {
        "name": trigger_name,
        "description": "A test event trigger for get",
        "listenFor": {"type": "test.event.type"},
        "type": "function",
        "function": function_name
    }

    create_response = requests.post(f"{base_url}/event-triggers", json=payload)
    assert_status_code(create_response, 201)

    # Now get it
    get_response = requests.get(f"{base_url}/event-triggers/{trigger_name}")
    assert_status_code(get_response, 200)

    trigger = get_response.json()
    assert trigger["name"] == trigger_name, "Trigger name does not match"

    # Clean up
    delete_trigger_url = f"{base_url}/event-triggers/{trigger_name}"
    del_trigger_response = requests.delete(delete_trigger_url)
    assert_status_code(del_trigger_response, 200)

    delete_function_url = f"{base_url}/functions/{function_name}"
    del_function_response = requests.delete(delete_function_url)
    assert_status_code(del_function_response, 200)

def test_list_event_triggers_by_event_type(base_url):
    """Test listing event triggers by event type."""
    # First create a function to reference
    function_name = "test-function-" + str(uuid.uuid4())[:8]
    function_payload = {
        "name": function_name,
        "description": "A test function",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
        "actionType": "chain",
        "actionTarget": "test-chain"
    }

    function_response = requests.post(f"{base_url}/functions", json=function_payload)
    assert_status_code(function_response, 201)

    # Create an event trigger with a specific event type
    event_type = "test.special.event"
    trigger_name = "test-event-type-trigger-" + str(uuid.uuid4())[:8]
    payload = {
        "name": trigger_name,
        "description": "A test event trigger for event type filtering",
        "listenFor": {"type": event_type},
        "type": "function",
        "function": function_name
    }

    create_response = requests.post(f"{base_url}/event-triggers", json=payload)
    assert_status_code(create_response, 201)

    # Now list by event type
    get_response = requests.get(f"{base_url}/event-triggers/event-type/{event_type}")
    assert_status_code(get_response, 200)

    triggers = get_response.json()
    assert isinstance(triggers, list), "Expected list of triggers"
    assert any(t["name"] == trigger_name for t in triggers), "Created trigger not found in filtered list"

    # Clean up
    delete_trigger_url = f"{base_url}/event-triggers/{trigger_name}"
    del_trigger_response = requests.delete(delete_trigger_url)
    assert_status_code(del_trigger_response, 200)

    delete_function_url = f"{base_url}/functions/{function_name}"
    del_function_response = requests.delete(delete_function_url)
    assert_status_code(del_function_response, 200)

def test_list_event_triggers_by_function(base_url):
    """Test listing event triggers by function name."""
    # First create a function to reference
    function_name = "test-function-" + str(uuid.uuid4())[:8]
    function_payload = {
        "name": function_name,
        "description": "A test function",
        "scriptType": "goja",
        "script": "function handler(input) { return input; }",
        "actionType": "chain",
        "actionTarget": "test-chain"
    }

    function_response = requests.post(f"{base_url}/functions", json=function_payload)
    assert_status_code(function_response, 201)

    # Create an event trigger with the function
    trigger_name = "test-function-trigger-" + str(uuid.uuid4())[:8]
    payload = {
        "name": trigger_name,
        "description": "A test event trigger for function filtering",
        "listenFor": {"type": "test.event.type"},
        "type": "function",
        "function": function_name
    }

    create_response = requests.post(f"{base_url}/event-triggers", json=payload)
    assert_status_code(create_response, 201)

    # Now list by function
    get_response = requests.get(f"{base_url}/event-triggers/function/{function_name}")
    assert_status_code(get_response, 200)

    triggers = get_response.json()
    assert isinstance(triggers, list), "Expected list of triggers"
    assert any(t["name"] == trigger_name for t in triggers), "Created trigger not found in filtered list"

    # Clean up
    delete_trigger_url = f"{base_url}/event-triggers/{trigger_name}"
    del_trigger_response = requests.delete(delete_trigger_url)
    assert_status_code(del_trigger_response, 200)

    delete_function_url = f"{base_url}/functions/{function_name}"
    del_function_response = requests.delete(delete_function_url)
    assert_status_code(del_function_response, 200)

def test_event_trigger_validation(base_url):
    """Test that event trigger validation works correctly."""
    # Test missing required fields
    invalid_payloads = [
        {"description": "Missing name"},  # Missing name
        {"name": "test-trigger"},  # Missing listenFor
        {"name": "test-trigger", "listenFor": {}},  # Missing listenFor.type
        {"name": "test-trigger", "listenFor": {"type": "test.event"}, "type": "invalid"},  # Invalid type
        {"name": "test-trigger", "listenFor": {"type": "test.event"}, "type": "function"},  # Missing function
    ]

    for payload in invalid_payloads:
        response = requests.post(f"{base_url}/event-triggers", json=payload)
        assert response.status_code == 400 or response.status_code == 422, f"Expected validation error for {payload}"

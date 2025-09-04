import requests
from helpers import assert_status_code
import uuid
import json
from conftest import check_langserve_direct_request

def test_langserve_direct_protocol(
    base_url,
    configurable_mock_hook_server
):
    """Test execution with LangServe Direct protocol"""
    # Setup mock server for LangServe Direct
    expected_hook_response = {"status": "ok", "data": "LangServe Direct executed"}

    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json=expected_hook_response,
        request_validator=check_langserve_direct_request
    )

    # Create a remote hook with LangServe Direct protocol
    hook_name = f"test-langserve-{uuid.uuid4().hex[:8]}"
    endpoint = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")
    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint,
            "method": "POST",
            "timeoutMs": 5000,
            "protocolType": "langserve"
        }
    )
    assert_status_code(create_response, 201)

    # Define a task chain that uses the hook
    task_chain = {
        "id": "langserve-test-chain",
        "debug": True,
        "description": "Test chain with LangServe Direct protocol",
        "tasks": [
            {
                "id": "hook_task",
                "handler": "hook",
                "hook": {
                    "name": hook_name,
                    "args": {
                        "param1": "value1",
                        "param2": "value2"
                    }
                },
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    # Execute the task chain
    response = requests.post(
        f"{base_url}/tasks",
        json={
            "input": "Trigger LangServe Direct hook",
            "chain": task_chain,
            "inputType": "string"
        }
    )
    assert_status_code(response, 200)

    # Verify the final response
    data = response.json()
    assert "output" in data, "Response missing output field"
    assert data["output"] == expected_hook_response, "Unexpected hook output"

    # Verify task execution history (state)
    assert len(data["state"]) == 1, "Should have one task in state"
    hook_task_state = data["state"][0]

    assert hook_task_state["taskHandler"] == "hook", "Wrong task handler"
    assert hook_task_state["inputType"] == "string", "Wrong input type"
    assert hook_task_state["outputType"] == "json", "Wrong output type"
    assert json.loads(hook_task_state["output"]) == expected_hook_response, "Task output mismatch"
    assert hook_task_state["transition"] == "ok", "Task transition mismatch"

    # Verify the mock server was called correctly
    assert len(mock_server["server"].log) > 0, "Mock server not called"
    request = mock_server["server"].log[0][0]
    request_data = request.json

    # In LangServe Direct, parameters are sent directly
    assert "name" not in request_data, "LangServe Direct should not have 'name' field"
    assert "arguments" not in request_data, "LangServe Direct should not have 'arguments' field"
    assert request_data["param1"] == "value1", "Hook arg mismatch"
    assert request_data["param2"] == "value2", "Hook arg mismatch"
    assert request_data["input"] == "Trigger LangServe Direct hook", "Input mismatch"

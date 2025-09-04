import requests
from helpers import assert_status_code
import uuid
import json
from conftest import check_ollama_request

def test_ollama_protocol(
    base_url,
    configurable_mock_hook_server
):
    """Test execution with Ollama protocol"""
    # Setup mock server for Ollama
    expected_hook_response = {"status": "ok", "data": "Ollama executed"}

    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json={
            "message": {
                "content": expected_hook_response
            }
        },  # Ollama-style response
        request_validator=check_ollama_request
    )

    # Create a remote hook with Ollama protocol
    hook_name = f"test-ollama-{uuid.uuid4().hex[:8]}"
    endpoint = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")
    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint,
            "method": "POST",
            "timeoutMs": 5000,
            "protocolType": "ollama"
        }
    )
    assert_status_code(create_response, 201)

    # Define a task chain that uses the hook
    task_chain = {
        "id": "ollama-test-chain",
        "debug": True,
        "description": "Test chain with Ollama protocol",
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
            "input": "Trigger Ollama hook",
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

    # In Ollama protocol, arguments are sent as an object (not a string)
    # The 'check_ollama_request' validator already confirms the presence of "name"
    # and that "arguments" is a dict. Here, we just check the content.
    assert request_data["name"] == hook_name, "Hook name mismatch"
    assert isinstance(request_data["arguments"], dict), "Arguments should be an object in Ollama protocol"

    arguments = request_data["arguments"]
    assert arguments["param1"] == "value1", "Hook arg mismatch"
    assert arguments["param2"] == "value2", "Hook arg mismatch"
    assert arguments["input"] == "Trigger Ollama hook", "Input mismatch"

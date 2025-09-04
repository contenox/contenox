import requests
from helpers import assert_status_code
import uuid
import json
from conftest import check_langserve_openai_request

def test_langserve_openai_protocol(
    base_url,
    configurable_mock_hook_server
):
    """Test execution with LangServe OpenAI protocol"""
    # Setup mock server for LangServe OpenAI
    expected_hook_response = {"status": "ok", "data": "LangServe OpenAI executed"}

    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json={"output": expected_hook_response},  # LangServe wraps output
        request_validator=check_langserve_openai_request
    )

    # Create a remote hook with LangServe OpenAI protocol
    hook_name = f"test-langserve-openai-{uuid.uuid4().hex[:8]}"
    endpoint = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")
    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint,
            "method": "POST",
            "timeoutMs": 5000,
            "protocolType": "langserve-openai"
        }
    )
    assert_status_code(create_response, 201)

    # Define a task chain that uses the hook
    task_chain = {
        "id": "langserve-openai-test-chain",
        "debug": True,
        "description": "Test chain with LangServe OpenAI protocol",
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
            "input": "Trigger LangServe OpenAI hook",
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

    # In LangServe OpenAI, parameters are sent in OpenAI format
    assert request_data["name"] == hook_name, "Hook name mismatch"

    # Arguments are a JSON string that needs parsing
    arguments = json.loads(request_data["arguments"])
    assert arguments["param1"] == "value1", "Hook arg mismatch"
    assert arguments["param2"] == "value2", "Hook arg mismatch"
    assert arguments["input"] == "Trigger LangServe OpenAI hook", "Input mismatch"

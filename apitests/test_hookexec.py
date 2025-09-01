import requests
from helpers import assert_status_code
import uuid
import json
from conftest import check_function_call_request

def test_hook_task_in_chain(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_group,
    create_backend_and_assign_to_group,
    wait_for_model_in_backend,
    configurable_mock_hook_server
):
    # Setup model and backend
    model_info = create_model_and_assign_to_group
    backend_info = create_backend_and_assign_to_group
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    # The mock server now returns a direct JSON object
    expected_hook_response = {"status": "ok", "data": "Hook executed successfully"}

    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json=expected_hook_response,
        request_validator=check_function_call_request
    )

    # Create a remote hook
    hook_name = f"test-hook-{uuid.uuid4().hex[:8]}"
    endpoint = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")
    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint,
            "method": "POST",
            "timeoutMs": 5000
        }
    )
    assert_status_code(create_response, 201)

    # Define a task chain that uses the hook
    task_chain = {
        "id": "hook-test-chain",
        "debug": True,
        "description": "Test chain with hook execution",
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
            "input": "Trigger hook",
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
    # The output type from a successful hook is now always 'json'
    assert hook_task_state["outputType"] == "json", "Wrong output type"
    # The output in the state is a string representation of the JSON object
    assert json.loads(hook_task_state["output"]) == expected_hook_response, "Task output mismatch"
    # The default transition for a JSON object output is 'ok'
    assert hook_task_state["transition"] == "ok", "Task transition mismatch"

    # Verify the mock server was called correctly
    assert len(mock_server["server"].log) > 0, "Mock server not called"
    request = mock_server["server"].log[0][0]
    request_data = request.json

    assert request_data["name"] == hook_name, "Hook name mismatch"

    # Arguments are now a JSON string, so we parse it to verify
    arguments = json.loads(request_data["arguments"])
    assert arguments["param1"] == "value1", "Hook arg mismatch"
    assert arguments["param2"] == "value2", "Hook arg mismatch"
    assert arguments["input"] == "Trigger hook", "Input mismatch"

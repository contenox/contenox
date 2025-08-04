import requests
from helpers import assert_status_code
import uuid
from conftest import MOCK_HOOK_RESPONSE

def test_hook_task_in_chain(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend,
    mock_hook_server
):
    # Setup model and backend
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    # Create a remote hook
    hook_name = f"test-hook-{uuid.uuid4().hex[:8]}"
    endpoint = mock_hook_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")
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
                    "branches": [
                        {
                            "operator": "default",
                            "goto": "end"
                        }
                    ]
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

    # Verify the response
    data = response.json()
    assert "response" in data, "Response missing response field"
    assert data["response"] == MOCK_HOOK_RESPONSE["output"], "Unexpected hook output"

    # Verify task execution history
    assert len(data["state"]) == 1, "Should have one task in state"
    hook_task = next(t for t in data["state"] if t["taskID"] == "hook_task")
    print(data)

    assert hook_task["taskHandler"] == "hook", "Wrong task handler"
    assert hook_task["inputType"] == "string", "Wrong input type"
    assert hook_task["outputType"] == "string", "Wrong output type"
    assert hook_task["output"] == MOCK_HOOK_RESPONSE["output"], "Task output mismatch"
    assert hook_task["transition"] == "ok", "Task transition mismatch"

    # Verify the mock server was called
    assert len(mock_hook_server["server"].log) > 0, "Mock server not called"
    request = mock_hook_server["server"].log[0][0]
    request_data = request.json
    assert request_data["args"]["name"] == hook_name, "Hook name mismatch"
    assert request_data["args"]["args"] == {"param1": "value1", "param2": "value2"}, "Hook args mismatch"
    assert request_data["input"] == "Trigger hook", "Input mismatch"
    assert request_data["dataType"] == "string", "DataType mismatch"

import uuid
import requests

def test_remote_hook_echo_without_headers(
    base_url,
    auth_headers
):
    """Test that the echo remote hook works correctly without custom headers"""
    input_value = "Test echo without custom headers"
    tool_name = "echo"  # Matches operationId in OpenAPI spec

    # Create the hook pointing to the echo service
    hook_name = f"test-echo-hook-{uuid.uuid4().hex[:8]}"
    endpoint_url = "http://echo:8000"  # From runtime's perspective within Docker network

    # Create the hook
    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint_url,
            "timeoutMs": 5000,
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    # Verify the hook was created correctly
    get_response = requests.get(
        f"{base_url}/hooks/remote/{create_response.json()['id']}",
        headers=auth_headers
    )
    assert get_response.status_code == 200
    hook = get_response.json()
    assert "headers" not in hook or not hook["headers"]

    # Define a task chain that uses the echo hook
    task_chain = {
        "id": "echo-no-headers-test-chain",
        "debug": True,
        "description": "Test chain with echo hook execution",
        "tasks": [{
            "id": "echo_hook_task",
            "handler": "hook",
            "hook": {
                "name": hook_name,
                "tool_name": tool_name,
                "args": {"input": input_value}
            },
            "transition": {
                "branches": [{"operator": "default", "goto": "end"}]
            }
        }],
        "token_limit": 4096
    }

    # Execute the task chain
    response = requests.post(
        f"{base_url}/tasks",
        json={
            "input": input_value,
            "chain": task_chain,
            "inputType": "string"
        },
        headers=auth_headers
    )

    # Verify the response indicates success
    assert response.status_code == 200
    data = response.json()
    assert "output" in data

    assert data["output"]["output"] == input_value

    assert len(data["state"]) == 1, "Should have one task in state"
    hook_task_state = data["state"][0]
    assert hook_task_state["taskHandler"] == "hook", "Wrong task handler"
    assert hook_task_state["inputType"] == "string", "Wrong input type"
    assert hook_task_state["outputType"] == "json", "Wrong output type"
    task_output = hook_task_state["output"]
    assert task_output == f'{{"output":"{input_value}"}}'
    assert hook_task_state["transition"] == "ok", "Task transition mismatch"


def test_remote_hook_echo_with_headers(
    base_url,
    auth_headers
):
    """Test that the echo remote hook works correctly with custom headers"""
    input_value = "Test echo with custom headers"
    tool_name = "echo"

    expected_headers = {
        "X-GitHub-Access-Token": "ghp_test1234567890",
        "X-Custom-Header": "custom-value",
        "Authorization": "Bearer token123"
    }

    # Create the hook pointing to the echo service
    hook_name = f"test-echo-headers-hook-{uuid.uuid4().hex[:8]}"
    endpoint_url = "http://echo:8000"  # From runtime's perspective

    # Create the hook with custom headers
    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint_url,
            "timeoutMs": 5000,
            "headers": expected_headers,
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    # Verify the hook was created correctly
    get_response = requests.get(
        f"{base_url}/hooks/remote/{create_response.json()['id']}",
        headers=auth_headers
    )
    assert get_response.status_code == 200
    hook = get_response.json()
    assert hook["headers"] == expected_headers

    # Define a task chain that uses the echo hook
    task_chain = {
        "id": "echo-headers-test-chain",
        "debug": True,
        "description": "Test chain with echo hook execution with headers",
        "tasks": [{
            "id": "echo_header_hook_task",
            "handler": "hook",
            "hook": {
                "name": hook_name,
                "tool_name": tool_name,
                "args": {"input": input_value}
            },
            "transition": {
                "branches": [{"operator": "default", "goto": "end"}]
            }
        }],
        "token_limit": 4096
    }

    # Execute the task chain
    response = requests.post(
        f"{base_url}/tasks",
        json={
            "input": input_value,
            "chain": task_chain,
            "inputType": "string"
        },
        headers=auth_headers
    )

    # Verify the response indicates success
    assert response.status_code == 200
    data = response.json()
    assert "output" in data

    # Verify the echo service returned our input value
    assert data["output"]["output"] == input_value

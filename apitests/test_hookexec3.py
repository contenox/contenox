import pytest
import uuid
import requests
import json
from conftest import BASE_URL, API_TOKEN, auth_headers, check_function_call_request


def test_remote_hook_with_headers(
    base_url,
    auth_headers,
    configurable_mock_hook_server
):
    """Test that remote hooks can define custom headers that are sent to the target endpoint"""
    expected_headers = {
        "X-GitHub-Access-Token": "ghp_test1234567890",
        "X-Custom-Header": "custom-value",
        "Authorization": "Bearer token123"
    }

    # The hook server now returns a direct JSON object.
    expected_response_json = {
        "status": "ok",
        "message": "Hook executed successfully with correct headers"
    }

    # Set up mock server with header and request format validation.
    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json=expected_response_json,
        expected_headers=expected_headers,
        request_validator=check_function_call_request
    )

    # Create a remote hook with custom headers
    hook_name = f"test-headers-hook-{uuid.uuid4().hex[:8]}"
    endpoint_url = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")

    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint_url,
            "method": "POST",
            "timeoutMs": 5000,
            "headers": expected_headers,
            "protocolType": "openai"
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    get_response = requests.get(
        f"{base_url}/hooks/remote/{create_response.json()['id']}",
        headers=auth_headers
    )
    assert get_response.status_code == 200
    hook = get_response.json()
    assert hook["headers"] == expected_headers

    # Define a task chain that uses the hook
    task_chain = {
        "id": "headers-test-chain",
        "debug": True,
        "description": "Test chain with header validation",
        "tasks": [
            {
                "id": "header_hook_task",
                "handler": "hook",
                "hook": {
                    "name": hook_name,
                    "args": {"test_param": "test_value"}
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
            "input": "Test header validation",
            "chain": task_chain,
            "inputType": "string"
        },
        headers=auth_headers
    )

    # Verify the response indicates success
    assert response.status_code == 200
    data = response.json()
    assert "output" in data
    # The final output of the chain is the direct JSON response from the hook
    assert data["output"] == expected_response_json

    # Verify the mock server was called
    assert len(mock_server["server"].log) > 0, "Mock server not called"


def test_remote_hook_header_validation_failure(
    base_url,
    auth_headers,
    configurable_mock_hook_server
):
    """Test that header validation failures are properly handled"""
    expected_headers = {"X-Required-Header": "required-value"}

    # Mock server will return 400 if headers don't match
    mock_server = configurable_mock_hook_server(
        status_code=400,
        response_json={"error": "Header validation failed"},
        expected_headers=expected_headers
    )

    hook_name = f"test-header-failure-{uuid.uuid4().hex[:8]}"
    endpoint_url = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")

    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint_url,
            "method": "POST",
            "timeoutMs": 5000,
            "protocolType": "openai",
            "headers": {"X-Different-Header": "different-value"}  # Mismatch
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    task_chain = {
        "id": "header-failure-test-chain",
        "debug": True,
        "tasks": [{
            "id": "header_hook_task",
            "handler": "hook",
            "hook": {"name": hook_name, "args": {}},
            "transition": {"branches": [{"operator": "default", "goto": "end"}]}
        }],
        "token_limit": 4096
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": "Test header validation failure", "chain": task_chain, "inputType": "string"},
        headers=auth_headers
    )

    # The task engine should propagate the failure from the hook
    assert response.status_code == 500
    data = response.json()
    assert "error" in data


def test_remote_hook_without_headers(
    base_url,
    auth_headers,
    configurable_mock_hook_server
):
    """Test that remote hooks work correctly without custom headers"""
    expected_response_json = {"message": "Hook executed successfully without custom headers"}

    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json=expected_response_json,
        request_validator=check_function_call_request
    )

    hook_name = f"test-no-headers-hook-{uuid.uuid4().hex[:8]}"
    endpoint_url = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")

    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint_url,
            "method": "POST",
            "timeoutMs": 5000,
            "protocolType": "openai",
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    get_response = requests.get(
        f"{base_url}/hooks/remote/{create_response.json()['id']}",
        headers=auth_headers
    )
    assert get_response.status_code == 200
    hook = get_response.json()
    assert "headers" not in hook or not hook["headers"]

    task_chain = {
        "id": "no-headers-test-chain",
        "debug": True,
        "tasks": [{
            "id": "no_header_hook_task",
            "handler": "hook",
            "hook": {"name": hook_name, "args": {}},
            "transition": {"branches": [{"operator": "default", "goto": "end"}]}
        }],
        "token_limit": 4096
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": "Test without custom headers", "chain": task_chain, "inputType": "string"},
        headers=auth_headers
    )

    assert response.status_code == 200
    data = response.json()
    assert "output" in data
    assert data["output"] == expected_response_json
    assert len(mock_server["server"].log) > 0, "Mock server not called"

import pytest
import uuid
import requests
import json
from conftest import BASE_URL, API_TOKEN, auth_headers


def test_remote_hook_with_headers(
    base_url,
    auth_headers,
    configurable_mock_hook_server
):
    """Test that remote hooks can define custom headers that are sent to the target endpoint"""

    # Define expected headers
    expected_headers = {
        "X-GitHub-Access-Token": "ghp_test1234567890",
        "X-Custom-Header": "custom-value",
        "Authorization": "Bearer token123"
    }

    # Set up mock server with header validation
    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json={
            "output": "Hook executed successfully with correct headers",
            "dataType": "string",
            "transition": "success"
        },
        expected_headers=expected_headers
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
            "headers": expected_headers
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    # Verify the hook was created with headers (if supported)
    get_response = requests.get(
        f"{base_url}/hooks/remote/{create_response.json()['id']}",
        headers=auth_headers
    )
    assert get_response.status_code == 200
    hook = get_response.json()

    # Check if headers field is supported
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
                    "args": {
                        "test_param": "test_value"
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
    assert data["output"] == "Hook executed successfully with correct headers"

    # Verify the mock server was called
    assert len(mock_server["server"].log) > 0, "Mock server not called"


def test_remote_hook_header_validation_failure(
    base_url,
    auth_headers,
    configurable_mock_hook_server
):
    """Test that header validation failures are properly handled"""

    # Define expected headers
    expected_headers = {
        "X-Required-Header": "required-value"
    }

    # Set up mock server with header validation that will fail
    mock_server = configurable_mock_hook_server(
        status_code=400,  # Will return 400 if headers don't match
        response_json={
            "error": "Header validation failed"
        },
        expected_headers=expected_headers
    )

    # Create a remote hook with different headers than expected
    hook_name = f"test-header-failure-{uuid.uuid4().hex[:8]}"
    endpoint_url = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")

    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint_url,
            "method": "POST",
            "timeoutMs": 5000,
            "headers": {
                "X-Different-Header": "different-value"  # Different from expected
            }
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    # Define a task chain that uses the hook
    task_chain = {
        "id": "header-failure-test-chain",
        "debug": True,
        "description": "Test chain with header validation failure",
        "tasks": [
            {
                "id": "header_hook_task",
                "handler": "hook",
                "hook": {
                    "name": hook_name,
                    "args": {
                        "test_param": "test_value"
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
            "input": "Test header validation failure",
            "chain": task_chain,
            "inputType": "string"
        },
        headers=auth_headers
    )

    # Verify the response indicates failure
    assert response.status_code == 500
    data = response.json()
    assert "error" in data


def test_remote_hook_without_headers(
    base_url,
    auth_headers,
    configurable_mock_hook_server
):
    """Test that remote hooks work correctly without custom headers"""

    # Set up mock server without header validation
    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json={
            "output": "Hook executed successfully without custom headers",
            "dataType": "string",
            "transition": "success"
        }
        # No expected_headers parameter means no header validation
    )

    # Create a remote hook without custom headers
    hook_name = f"test-no-headers-hook-{uuid.uuid4().hex[:8]}"
    endpoint_url = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")

    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint_url,
            "method": "POST",
            "timeoutMs": 5000
            # No headers field
        },
        headers=auth_headers
    )
    assert create_response.status_code == 201

    # Verify the hook was created without headers
    get_response = requests.get(
        f"{base_url}/hooks/remote/{create_response.json()['id']}",
        headers=auth_headers
    )
    assert get_response.status_code == 200
    hook = get_response.json()
    # Headers should be either missing or empty
    assert "headers" not in hook or not hook["headers"]

    # Define a task chain that uses the hook
    task_chain = {
        "id": "no-headers-test-chain",
        "debug": True,
        "description": "Test chain without custom headers",
        "tasks": [
            {
                "id": "no_header_hook_task",
                "handler": "hook",
                "hook": {
                    "name": hook_name,
                    "args": {
                        "test_param": "test_value"
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
            "input": "Test without custom headers",
            "chain": task_chain,
            "inputType": "string"
        },
        headers=auth_headers
    )

    # Verify the response indicates success
    assert response.status_code == 200
    data = response.json()
    assert "output" in data
    assert data["output"] == "Hook executed successfully without custom headers"

    # Verify the mock server was called
    assert len(mock_server["server"].log) > 0, "Mock server not called"

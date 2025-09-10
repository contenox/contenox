import requests
from helpers import assert_status_code
import uuid
from urllib.parse import urlparse

def test_remote_hook_schema_fetching(
    base_url,
    configurable_mock_hook_server,
    auth_headers
):
    """Test that the system handles schema fetching correctly for remote hooks"""
    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json={"status": "ok"},
    )

    # Get the path from the mock server's URL
    parsed_mock_url = urlparse(mock_server["url"])
    base_url_mock = f"http://host.docker.internal:{parsed_mock_url.port}"
    path = mock_server["base_path"]  # Use base_path instead of path
    endpoint = base_url_mock + path

    hook_name = f"test-schema-hook-{uuid.uuid4().hex[:8]}"

    # Define a comprehensive OpenAPI schema
    expected_schema = {
        "openapi": "3.0.0",
        "info": {
            "title": "Comprehensive Test API",
            "version": "1.0.0",
            "description": "This is a test API for schema fetching."
        },
        "paths": {
            "/test-operation": {
                "post": {
                    "operationId": "testOperation",
                    "summary": "Test Operation Summary",
                    "description": "This is a detailed description of the test operation.",
                    "requestBody": {
                        "required": True,
                        "content": {
                            "application/json": {
                                "schema": {
                                    "type": "object",
                                    "properties": {
                                        "input": {
                                            "type": "string",
                                            "description": "The main input string for the operation."
                                        },
                                        "config": {
                                            "type": "object",
                                            "properties": {
                                                "timeout": {
                                                    "type": "integer",
                                                    "minimum": 1,
                                                    "description": "Timeout in seconds."
                                                }
                                            }
                                        }
                                    },
                                    "required": ["input"]
                                }
                            }
                        }
                    },
                    "responses": {
                        "200": {
                            "description": "Successful response",
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "type": "object",
                                        "properties": {
                                            "result": {"type": "string"},
                                            "duration": {"type": "number"}
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    mock_server["server"].expect_request(
        f"{path}/openapi.json",
        method="GET"
    ).respond_with_json(expected_schema)

    # Create a remote hook
    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint,
            "timeoutMs": 5000,
        },
        headers=auth_headers
    )
    assert_status_code(create_response, 201)

    # Verify the schema was fetched correctly by checking /hooks/schemas
    schemas_response = requests.get(
        f"{base_url}/hooks/schemas",
        headers=auth_headers
    )
    assert_status_code(schemas_response, 200)
    schemas = schemas_response.json()

    # Check if our hook's schema is in the response
    assert hook_name in schemas, f"Schema for {hook_name} should be available"
    # Compare the entire schema object
    assert schemas[hook_name] == expected_schema, "Schema should match what the mock server provided"

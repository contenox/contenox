import requests
from helpers import assert_status_code
import uuid
from urllib.parse import urlparse  # Add this import

def test_remote_hook_schema_fetching(
    base_url,
    configurable_mock_hook_server,
    auth_headers
):
    """Test that the system handles schema fetching correctly for remote hooks"""
    # Configure mock server to serve BOTH the main endpoint AND schema endpoint
    mock_server = configurable_mock_hook_server(
        status_code=200,
        response_json={"status": "ok"},
        # We'll set up the schema endpoint separately
    )

    # Add schema endpoint to the mock server
    # Get the path from the mock server's URL
    parsed_url = urlparse(mock_server["url"])
    endpoint_path = parsed_url.path
    schema_endpoint = endpoint_path + "/schema"  # Append /schema to the endpoint path

    expected_schema = {
        "name": "test-hook",
        "description": "Test hook schema",
        "parameters": {
            "type": "object",
            "properties": {
                "input": {"type": "string", "description": "Input data"}
            },
            "required": ["input"]
        }
    }

    # Configure schema endpoint
    mock_server["server"].expect_request(
        schema_endpoint,
        method="GET"
    ).respond_with_json(expected_schema)

    # Create a remote hook pointing to our mock server
    hook_name = f"test-schema-hook-{uuid.uuid4().hex[:8]}"
    endpoint = mock_server["url"].replace("http://0.0.0.0", "http://host.docker.internal")

    create_response = requests.post(
        f"{base_url}/hooks/remote",
        json={
            "name": hook_name,
            "endpointUrl": endpoint,
            "method": "POST",
            "timeoutMs": 5000,
            "protocolType": "openai"
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
    assert schemas[hook_name] == expected_schema, "Schema should match what the mock server provided"

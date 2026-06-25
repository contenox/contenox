from helpers import api_url, assert_status_code


def test_local_tools_endpoint(api, base_url):
    response = api.get(api_url(base_url, "tools", "local"), timeout=15)
    assert_status_code(response, 200)
    tools = response.json()
    assert isinstance(tools, list)
    names = {item.get("name") for item in tools}
    assert "local_fs" in names
    assert "local_shell" in names


def test_tool_schemas_endpoint(api, base_url):
    response = api.get(api_url(base_url, "tools", "schemas"), timeout=15)
    assert_status_code(response, 200)
    schemas = response.json()
    assert isinstance(schemas, dict)

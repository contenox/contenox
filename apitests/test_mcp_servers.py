from helpers import api_url, assert_status_code, assert_status_in, unique_name


def _mcp_payload(name: str, transport: str = "sse") -> dict:
    if transport == "stdio":
        return {
            "name": name,
            "transport": "stdio",
            "command": "npx",
            "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
            "connectTimeoutSeconds": 30,
        }
    return {
        "name": name,
        "transport": transport,
        "url": f"http://example.invalid/{name}/mcp",
        "connectTimeoutSeconds": 30,
    }


def test_mcp_server_crud(api, base_url):
    name = unique_name("apitest-mcp")
    response = api.post(api_url(base_url, "mcp-servers"), json=_mcp_payload(name), timeout=15)
    assert_status_code(response, 201)
    server = response.json()
    server_id = server["id"]

    try:
        response = api.get(api_url(base_url, "mcp-servers", server_id), timeout=15)
        assert_status_code(response, 200)
        assert response.json()["name"] == name

        response = api.get(api_url(base_url, "mcp-servers", "by-name", name), timeout=15)
        assert_status_code(response, 200)
        assert response.json()["id"] == server_id

        updated = _mcp_payload(name, transport="http")
        updated["headers"] = {"X-Test": "apitest"}
        updated["injectParams"] = {"workspace": "local"}
        response = api.put(api_url(base_url, "mcp-servers", server_id), json=updated, timeout=15)
        assert_status_code(response, 200)
        body = response.json()
        assert body["transport"] == "http"
        assert body["headers"]["X-Test"] == "apitest"
        assert body["injectParams"]["workspace"] == "local"

        response = api.get(api_url(base_url, "mcp-servers"), timeout=15)
        assert_status_code(response, 200)
        assert any(item["id"] == server_id for item in response.json())
    finally:
        response = api.delete(api_url(base_url, "mcp-servers", server_id), timeout=15)
        assert_status_code(response, 200)

    response = api.get(api_url(base_url, "mcp-servers", server_id), timeout=15)
    assert_status_code(response, 404)


def test_mcp_server_accepts_stdio_transport(api, base_url):
    name = unique_name("apitest-mcp-stdio")
    response = api.post(api_url(base_url, "mcp-servers"), json=_mcp_payload(name, "stdio"), timeout=15)
    assert_status_code(response, 201)
    server_id = response.json()["id"]
    try:
        assert response.json()["transport"] == "stdio"
        assert response.json()["command"] == "npx"
    finally:
        response = api.delete(api_url(base_url, "mcp-servers", server_id), timeout=15)
        assert_status_code(response, 200)


def test_mcp_server_validation_rejects_unknown_transport(api, base_url):
    response = api.post(
        api_url(base_url, "mcp-servers"),
        json={"name": unique_name("apitest-mcp"), "transport": "ftp"},
        timeout=15,
    )
    assert_status_in(response, (400, 422))

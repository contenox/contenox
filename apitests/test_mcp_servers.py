import pytest
import requests
import uuid
from helpers import assert_status_code

BASE = "mcp-servers"

VALID_SSE = {
    "name": "test-mcp-sse",
    "transport": "sse",
    "url": "http://mcp-server.example.com/sse",
    "connectTimeoutSeconds": 30,
}

VALID_HTTP = {
    "name": "test-mcp-http",
    "transport": "http",
    "url": "http://mcp-server.example.com/mcp",
    "connectTimeoutSeconds": 30,
}

VALID_STDIO = {
    "name": "test-mcp-stdio",
    "transport": "stdio",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
    "connectTimeoutSeconds": 30,
}

INVALID_SERVERS = [
    ({**VALID_SSE, "name": ""},        "empty name"),
    ({**VALID_SSE, "transport": ""},   "empty transport"),
    ({**VALID_SSE, "transport": "ftp"},"unknown transport"),
    ({**VALID_SSE, "url": ""},         "sse missing url"),
    ({**VALID_HTTP, "url": ""},        "http missing url"),
    ({"name": "x", "transport": "stdio"},  "stdio missing command"),
]


def make_unique(base: dict, suffix: str = None) -> dict:
    suffix = suffix or str(uuid.uuid4())[:8]
    return {**base, "name": f"{base['name']}-{suffix}"}


# ─── CRUD happy path ───────────────────────────────────────────────────────────

def test_create_mcp_server_sse(base_url):
    """Test creating an SSE MCP server configuration."""
    payload = make_unique(VALID_SSE)
    r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(r, 201)
    data = r.json()
    assert "id" in data
    assert data["name"] == payload["name"]
    assert data["transport"] == "sse"
    assert data["url"] == payload["url"]


def test_create_mcp_server_http(base_url):
    """Test creating an HTTP MCP server configuration."""
    payload = make_unique(VALID_HTTP)
    r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(r, 201)
    data = r.json()
    assert data["transport"] == "http"


def test_create_mcp_server_stdio(base_url):
    """Test creating a stdio MCP server configuration."""
    payload = make_unique(VALID_STDIO)
    r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(r, 201)
    data = r.json()
    assert data["transport"] == "stdio"
    assert data["command"] == "npx"
    assert "-y" in data["args"]


def test_get_mcp_server_by_id(base_url):
    """Test retrieving an MCP server configuration by its unique ID."""
    payload = make_unique(VALID_SSE)
    create_r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(create_r, 201)
    srv_id = create_r.json()["id"]

    get_r = requests.get(f"{base_url}/{BASE}/{srv_id}")
    assert_status_code(get_r, 200)
    data = get_r.json()
    assert data["id"] == srv_id
    assert data["name"] == payload["name"]


def test_get_mcp_server_by_name(base_url):
    """Test retrieving an MCP server configuration by its unique name."""
    payload = make_unique(VALID_SSE)
    create_r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(create_r, 201)
    srv_id = create_r.json()["id"]

    get_r = requests.get(f"{base_url}/{BASE}/by-name/{payload['name']}")
    assert_status_code(get_r, 200)
    data = get_r.json()
    assert data["id"] == srv_id
    assert data["name"] == payload["name"]


def test_list_mcp_servers(base_url):
    """Test listing MCP server configurations."""
    # Create two servers with unique names
    s1 = make_unique(VALID_SSE, "list-a")
    s2 = make_unique(VALID_HTTP, "list-b")
    requests.post(f"{base_url}/{BASE}", json=s1)
    requests.post(f"{base_url}/{BASE}", json=s2)

    r = requests.get(f"{base_url}/{BASE}")
    assert_status_code(r, 200)
    items = r.json()
    assert isinstance(items, list)
    names = {i["name"] for i in items}
    assert s1["name"] in names
    assert s2["name"] in names


def test_update_mcp_server(base_url):
    """Test updating an existing MCP server configuration."""
    payload = make_unique(VALID_SSE)
    create_r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(create_r, 201)
    srv_id = create_r.json()["id"]

    updated = {
        **payload,
        "url": "http://new-mcp-server.example.com/sse",
        "connectTimeoutSeconds": 60,
    }
    update_r = requests.put(f"{base_url}/{BASE}/{srv_id}", json=updated)
    assert_status_code(update_r, 200)

    get_r = requests.get(f"{base_url}/{BASE}/{srv_id}")
    data = get_r.json()
    assert data["url"] == "http://new-mcp-server.example.com/sse"
    assert data["connectTimeoutSeconds"] == 60


def test_delete_mcp_server(base_url):
    """Test deleting an MCP server configuration."""
    payload = make_unique(VALID_SSE)
    create_r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(create_r, 201)
    srv_id = create_r.json()["id"]

    del_r = requests.delete(f"{base_url}/{BASE}/{srv_id}")
    assert_status_code(del_r, 200)

    get_r = requests.get(f"{base_url}/{BASE}/{srv_id}")
    assert_status_code(get_r, 404)


# ─── Validation failures ───────────────────────────────────────────────────────

@pytest.mark.parametrize("payload,description", INVALID_SERVERS)
def test_create_mcp_server_validation_failures(base_url, payload, description):
    """Test that invalid MCP server payloads are rejected."""
    r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert r.status_code in (400, 422), (
        f"Expected 400 or 422 for '{description}', got {r.status_code}: {r.text}"
    )


# ─── 404 scenarios ─────────────────────────────────────────────────────────────

def test_get_nonexistent_mcp_server_returns_404(base_url):
    """Test that fetching a non-existent MCP server ID returns 404."""
    r = requests.get(f"{base_url}/{BASE}/{uuid.uuid4()}")
    assert_status_code(r, 404)


def test_get_mcp_server_by_name_not_found(base_url):
    """Test that fetching a non-existent MCP server name returns 404."""
    r = requests.get(f"{base_url}/{BASE}/by-name/this-name-does-not-exist-{uuid.uuid4()}")
    assert_status_code(r, 404)


def test_update_nonexistent_mcp_server_returns_404(base_url):
    """Test that updating a non-existent MCP server returns 404."""
    r = requests.put(f"{base_url}/{BASE}/{uuid.uuid4()}", json=VALID_SSE)
    assert_status_code(r, 404)


def test_delete_nonexistent_mcp_server_returns_404(base_url):
    """Test that deleting a non-existent MCP server returns 404."""
    r = requests.delete(f"{base_url}/{BASE}/{uuid.uuid4()}")
    assert_status_code(r, 404)


# ─── Uniqueness ────────────────────────────────────────────────────────────────

def test_duplicate_name_returns_409(base_url):
    """Test that creating two MCP servers with the same name returns 409."""
    payload = make_unique(VALID_SSE)
    r1 = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(r1, 201)

    r2 = requests.post(f"{base_url}/{BASE}", json={**payload, "url": "http://other.example.com/sse"})
    assert_status_code(r2, 409)


# ─── Auth config round-trip ───────────────────────────────────────────────────

def test_create_mcp_server_with_bearer_auth(base_url):
    """Test creating an MCP server with bearer token auth stored via env key."""
    payload = {
        **make_unique(VALID_SSE),
        "authType": "bearer",
        "authEnvKey": "MY_MCP_TOKEN",
    }
    r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(r, 201)
    data = r.json()
    assert data["authType"] == "bearer"
    assert data["authEnvKey"] == "MY_MCP_TOKEN"


# ─── Inject params & headers ──────────────────────────────────────────────────

def test_create_mcp_server_with_inject_params(base_url):
    """Test creating an MCP server with injectParams stored and round-tripped."""
    payload = {
        **make_unique(VALID_HTTP),
        "injectParams": {"tenant_id": "acme", "env": "production"},
    }
    r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(r, 201)
    data = r.json()
    assert "injectParams" in data
    assert data["injectParams"]["tenant_id"] == "acme"
    assert data["injectParams"]["env"] == "production"


def test_create_mcp_server_with_headers(base_url):
    """Test creating an MCP server with additional HTTP headers stored and round-tripped."""
    payload = {
        **make_unique(VALID_SSE),
        "headers": {"X-Tenant": "acme", "X-Version": "2"},
    }
    r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(r, 201)
    data = r.json()
    assert "headers" in data
    assert data["headers"]["X-Tenant"] == "acme"
    assert data["headers"]["X-Version"] == "2"


def test_update_mcp_server_inject_params_replaces_all(base_url):
    """Test that updating injectParams via PUT replaces the entire map."""
    payload = {
        **make_unique(VALID_HTTP),
        "injectParams": {"tenant_id": "old", "extra": "gone"},
    }
    create_r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(create_r, 201)
    srv_id = create_r.json()["id"]

    updated = {**payload, "injectParams": {"tenant_id": "new"}}
    update_r = requests.put(f"{base_url}/{BASE}/{srv_id}", json=updated)
    assert_status_code(update_r, 200)

    get_r = requests.get(f"{base_url}/{BASE}/{srv_id}")
    data = get_r.json()
    assert data["injectParams"]["tenant_id"] == "new"
    assert "extra" not in data.get("injectParams", {})


def test_mcp_server_inject_params_persist_across_get(base_url):
    """Test that injectParams survive a create → get by ID round-trip."""
    payload = {
        **make_unique(VALID_SSE),
        "injectParams": {"correlation_id": "trace-xyz"},
    }
    create_r = requests.post(f"{base_url}/{BASE}", json=payload)
    assert_status_code(create_r, 201)
    srv_id = create_r.json()["id"]

    get_r = requests.get(f"{base_url}/{BASE}/{srv_id}")
    assert_status_code(get_r, 200)
    data = get_r.json()
    assert data["injectParams"]["correlation_id"] == "trace-xyz"

"""HTTP-level coverage for the declared-agents registry (runtime/internal/agentregistryapi).

The registry is deliberately READ-ONLY over REST in this slice: listing and
looking up declared agents is served here, while registration (create/update/
delete) stays with the `contenox agent` CLI. These tests pin both halves of that
contract — the read shapes and the read-only-ness — against a live `contenox
serve`.

## Why some tests skip in this base

Populating the registry without a CLI write surface depends on chain-agent
DISCOVERY (blueprint C9 / "chain plumbing"): a sibling slice that upserts each
agent-*.json chain as a declared agent at serve startup. That slice is not
present in this base, so the registry is empty here and GET /agents answers
`null`. The by-name/by-id roundtrip and the discovered-fixture check therefore
SKIP when the registry is empty, and activate automatically once discovery
lands (the harness already seeds apitests/fixtures/agent-apitest-noop.json for
exactly that moment — see scripts/run_apitests.sh). The read-only-ness and
unknown-id contracts hold in either base and always run.
"""

import pytest

from helpers import api_url, assert_status_code, assert_status_in


FIXTURE_AGENT = "agent-apitest-noop"


def _list_agents(api, base_url):
    response = api.get(api_url(base_url, "agents"), timeout=15)
    assert_status_code(response, 200)
    body = response.json()
    # Empty registry serializes as JSON null (nil slice) until discovery seeds
    # rows; coerce so callers can treat "empty" uniformly.
    assert body is None or isinstance(body, list)
    return body or []


def test_agents_list_shape(api, base_url):
    agents = _list_agents(api, base_url)
    for agent in agents:
        assert isinstance(agent["id"], str) and agent["id"]
        assert isinstance(agent["name"], str) and agent["name"]
        assert isinstance(agent["kind"], str) and agent["kind"]
        assert isinstance(agent["enabled"], bool)
        assert "configJson" in agent


def test_agents_get_by_name_and_by_id_roundtrip(api, base_url):
    agents = _list_agents(api, base_url)
    if not agents:
        pytest.skip("no declared agents in this base — agent discovery is a sibling slice")
    first = agents[0]

    by_name = api.get(api_url(base_url, "agents", "by-name", first["name"]), timeout=15)
    assert_status_code(by_name, 200)
    assert by_name.json()["name"] == first["name"]
    assert by_name.json()["id"] == first["id"]

    by_id = api.get(api_url(base_url, "agents", first["id"]), timeout=15)
    assert_status_code(by_id, 200)
    assert by_id.json()["id"] == first["id"]
    assert by_id.json()["name"] == first["name"]


def test_agents_fixture_agent_is_discovered(api, base_url):
    # The harness seeds apitests/fixtures/agent-apitest-noop.json before boot;
    # once chain-agent discovery lands, it becomes a kind "chain" agent named
    # FIXTURE_AGENT — the same agent test_fleet.py dispatches. Until then the
    # seed is inert (nothing discovers it) and this skips.
    response = api.get(api_url(base_url, "agents", "by-name", FIXTURE_AGENT), timeout=15)
    if response.status_code == 404:
        pytest.skip(
            f"{FIXTURE_AGENT} not discovered — chain-agent discovery is a sibling slice; "
            "the harness already seeds the fixture for when it lands"
        )
    assert_status_code(response, 200)
    body = response.json()
    assert body["name"] == FIXTURE_AGENT
    assert body["kind"] == "chain"
    assert body["enabled"] is True


def test_agents_get_by_name_unknown_is_404(api, base_url):
    response = api.get(
        api_url(base_url, "agents", "by-name", "no-such-agent-apitest"), timeout=15
    )
    assert_status_code(response, 404)


def test_agents_get_by_id_unknown_is_404(api, base_url):
    response = api.get(
        api_url(base_url, "agents", "00000000-0000-0000-0000-000000000000"), timeout=15
    )
    assert_status_code(response, 404)


def test_agents_registry_is_read_only(api, base_url):
    # No write surface over REST: create/update/delete stay with the CLI, so the
    # mux registers only GET handlers on these paths. An unregistered method is
    # 405; 404 is accepted too, so the test pins "no mutation succeeded" rather
    # than one exact rejection code.
    created = api.post(api_url(base_url, "agents"), json={"name": "x"}, timeout=15)
    assert_status_in(created, (404, 405))

    updated = api.put(
        api_url(base_url, "agents", "00000000-0000-0000-0000-000000000000"),
        json={"name": "x"},
        timeout=15,
    )
    assert_status_in(updated, (404, 405))

    deleted = api.delete(
        api_url(base_url, "agents", "00000000-0000-0000-0000-000000000000"), timeout=15
    )
    assert_status_in(deleted, (404, 405))

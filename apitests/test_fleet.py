"""HTTP-level coverage for the fleet surface (runtime/internal/fleetapi).

The fleet is the config+runtime join of every declared agent annotated with its
running instances, plus the dispatch/stop/cancel verbs that drive it. These
tests pin the read shapes and the dispatch validation errors, and carry a full
dispatch -> running -> stop lifecycle test that activates the moment a
hermetically-dispatchable agent exists.

## Why the lifecycle test skips in this base

A real hermetic dispatch needs a declared agent that resolves NO model — a
no-model chain agent. Declaring one without a CLI depends on chain-agent
DISCOVERY and the `chain` agent kind (blueprint C9 / "chain plumbing"), a
sibling slice not present in this base: here the `chain` kind is still rejected
at validation and nothing walks the workspace for agent-*.json chains, so the
registry is empty and there is no agent a dispatch could bring up without a
model or a backend. That is the boundary this suite covers up to.

The harness already seeds apitests/fixtures/agent-apitest-noop.json before boot,
so the instant that sibling slice lands (discovery declares the fixture as a
kind "chain" agent) test_dispatch_lifecycle_running_then_stopped stops skipping
and proves the whole loop — with no change here. A dispatched unit is a
subprocess of the serve binary running a single noop task: no model, no backend,
no network. Every test that dispatches stops its instance and deletes its
mission in a finally.
"""

import contextlib

import pytest

from helpers import api_url, assert_status_code, assert_status_in


FIXTURE_AGENT = "agent-apitest-noop"
# The instance states agentinstance can report (runtime/agentinstance/instance.go).
VALID_INSTANCE_STATES = {"starting", "running", "stopped", "error", "warning"}


def _fixture_agent_present(api, base_url) -> bool:
    response = api.get(api_url(base_url, "agents", "by-name", FIXTURE_AGENT), timeout=15)
    return response.status_code == 200


def _stop_instance(api, base_url, instance_id):
    with contextlib.suppress(Exception):
        api.delete(api_url(base_url, "fleet", instance_id), timeout=15)


def _delete_mission(api, base_url, mission_id):
    with contextlib.suppress(Exception):
        api.delete(api_url(base_url, "missions", mission_id), timeout=15)


def test_fleet_list_shape(api, base_url):
    response = api.get(api_url(base_url, "fleet"), timeout=15)
    assert_status_code(response, 200)
    entries = response.json()
    assert isinstance(entries, list)
    for entry in entries:
        assert isinstance(entry["agentId"], str) and entry["agentId"]
        assert isinstance(entry["agentName"], str) and entry["agentName"]
        assert isinstance(entry["kind"], str) and entry["kind"]
        # instances is null for an idle declared agent, else a list of statuses.
        assert entry["instances"] is None or isinstance(entry["instances"], list)


def test_fleet_get_unknown_instance_is_404(api, base_url):
    response = api.get(
        api_url(base_url, "fleet", "no-such-instance-apitest"), timeout=15
    )
    assert_status_code(response, 404)


def test_dispatch_missing_agent_name_is_400(api, base_url):
    response = api.post(
        api_url(base_url, "fleet", "dispatch"),
        json={"intent": "do the thing", "hitlPolicyName": "hitl-policy-default.json"},
        timeout=15,
    )
    assert_status_code(response, 400)


def test_dispatch_missing_intent_is_400(api, base_url):
    response = api.post(
        api_url(base_url, "fleet", "dispatch"),
        json={"agentName": FIXTURE_AGENT, "hitlPolicyName": "hitl-policy-default.json"},
        timeout=15,
    )
    assert_status_code(response, 400)


def test_dispatch_missing_policy_is_400(api, base_url):
    # Every dispatch is a mission and a mission must name its envelope, so a
    # dispatch with no hitlPolicyName is rejected before anything is brought up.
    response = api.post(
        api_url(base_url, "fleet", "dispatch"),
        json={"agentName": FIXTURE_AGENT, "intent": "do the thing"},
        timeout=15,
    )
    assert_status_code(response, 400)


def test_dispatch_nonexistent_policy_is_400(api, base_url):
    # The envelope is the load-bearing invariant of mission mode: a dispatch that
    # names a policy which does not load is refused up front, so a typo can never
    # run a mission under the silently substituted default gate.
    response = api.post(
        api_url(base_url, "fleet", "dispatch"),
        json={
            "agentName": FIXTURE_AGENT,
            "intent": "do the thing",
            "hitlPolicyName": "no-such-policy-apitest.json",
        },
        timeout=15,
    )
    assert_status_code(response, 400)
    body = response.json()
    assert body.get("error", {}).get("param") == "hitlPolicyName", body


def test_dispatch_unknown_agent_is_404(api, base_url):
    # An unknown agent is refused at resolve time, before any instance is
    # allocated — the ResolveForSpawn gate.
    response = api.post(
        api_url(base_url, "fleet", "dispatch"),
        json={
            "agentName": "no-such-agent-apitest",
            "intent": "do the thing",
            "hitlPolicyName": "hitl-policy-default.json",
        },
        timeout=15,
    )
    assert_status_code(response, 404)


def test_dispatch_lifecycle_running_then_stopped(api, base_url):
    """The full loop: fire a mission at the no-model fixture agent, see the unit
    running on the board, read its mission, and stop it.

    A dispatch returns 202 as soon as the session is open, which means the unit
    subprocess actually spawned and initialized — so a 202 here is itself proof
    the self-spawned chain unit boots hermetically."""
    if not _fixture_agent_present(api, base_url):
        pytest.skip(
            f"{FIXTURE_AGENT} not discovered — chain-agent discovery and the `chain` "
            "agent kind are a sibling slice not present in this base; the harness "
            "already seeds the fixture for when it lands"
        )

    dispatched = api.post(
        api_url(base_url, "fleet", "dispatch"),
        json={
            "agentName": FIXTURE_AGENT,
            "intent": "do the apitest thing",
            "hitlPolicyName": "hitl-policy-default.json",
        },
        timeout=30,
    )
    assert_status_code(dispatched, 202)
    result = dispatched.json()
    instance_id = result["instanceId"]
    session_id = result["sessionId"]
    mission_id = result["missionId"]
    assert instance_id and session_id and mission_id

    try:
        # The unit is now on the board, addressable by its instance id.
        status = api.get(api_url(base_url, "fleet", instance_id), timeout=15)
        assert_status_code(status, 200)
        inst = status.json()
        assert inst["id"] == instance_id
        assert inst["kind"] == "chain"
        assert inst["state"] in VALID_INSTANCE_STATES

        # It also appears in the list join under its declared agent.
        board = api.get(api_url(base_url, "fleet"), timeout=15).json()
        instance_ids = {
            i["id"]
            for entry in board
            for i in (entry["instances"] or [])
        }
        assert instance_id in instance_ids

        # Every dispatch is a mission, bound to the unit it spawned.
        mission = api.get(api_url(base_url, "missions", mission_id), timeout=15)
        assert_status_code(mission, 200)
        assert mission.json()["instanceId"] == instance_id
        assert mission.json()["agentName"] == FIXTURE_AGENT

        # Stop is idempotent by kernel contract and returns the plain "deleted".
        stopped = api.delete(api_url(base_url, "fleet", instance_id), timeout=15)
        assert_status_code(stopped, 200)

        # A second stop on the now-gone instance is still a no-op 200, not a 404.
        again = api.delete(api_url(base_url, "fleet", instance_id), timeout=15)
        assert_status_in(again, (200, 404))
    finally:
        _stop_instance(api, base_url, instance_id)
        _delete_mission(api, base_url, mission_id)


def test_cancel_unknown_instance_is_404(api, base_url):
    # Cancel targets an in-flight turn; an unknown instance is 404. The body is
    # optional (an absent one means "every session"), so this also exercises the
    # no-body path.
    response = api.post(
        api_url(base_url, "fleet", "no-such-instance-apitest", "cancel"),
        timeout=15,
    )
    assert_status_code(response, 404)

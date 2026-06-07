from helpers import api_url, assert_status_code, unique_name


def _policy_url(base_url: str, name: str) -> str:
    return api_url(base_url, "hitl-policies", name=name)


def test_hitl_policy_create_get_update_delete(api, base_url):
    name = f"hitl-policy-{unique_name('apitest')}.json"
    policy = {
        "default_action": "deny",
        "rules": [{"hook": "local_shell", "tool": "local_shell", "action": "approve"}],
    }

    response = api.post(_policy_url(base_url, name), json=policy, timeout=15)
    assert_status_code(response, 201)
    assert response.json()["default_action"] == "deny"

    response = api.get(_policy_url(base_url, name), timeout=15)
    assert_status_code(response, 200)
    assert response.json()["rules"][0]["action"] == "approve"

    updated = {
        "default_action": "allow",
        "rules": [{"hook": "local_fs", "tool": "read_file", "action": "allow"}],
    }
    response = api.put(_policy_url(base_url, name), json=updated, timeout=15)
    assert_status_code(response, 200)
    assert response.json()["default_action"] == "allow"

    response = api.delete(_policy_url(base_url, name), timeout=15)
    assert_status_code(response, 200)

    response = api.get(_policy_url(base_url, name), timeout=15)
    assert_status_code(response, 404)


def test_hitl_policy_list_endpoint(api, base_url):
    response = api.get(api_url(base_url, "hitl-policies/list"), timeout=15)
    assert_status_code(response, 200)
    assert isinstance(response.json(), list)


def test_hitl_policy_rejects_invalid_name(api, base_url):
    response = api.post(
        _policy_url(base_url, "not-a-policy.json"),
        json={"rules": []},
        timeout=15,
    )
    assert_status_code(response, 400)

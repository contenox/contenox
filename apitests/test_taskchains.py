from helpers import api_url, assert_status_code, unique_name


def _taskchain_path(base_url: str, path: str) -> str:
    return api_url(base_url, "taskchains", path=path)


def _chain(chain_id: str, description: str = "API smoke test chain") -> dict:
    return {
        "id": chain_id,
        "debug": False,
        "description": description,
        "token_limit": 4096,
        "tasks": [
            {
                "id": "start",
                "handler": "prompt_to_string",
                "prompt_template": "Hello from apitest",
                "transition": {
                    "branches": [{"operator": "default", "when": "", "goto": "end"}],
                },
            }
        ],
    }


def test_taskchain_create_get_update_delete(api, base_url):
    chain_id = unique_name("apitest-chain")
    path = f"{chain_id}.json"

    response = api.post(_taskchain_path(base_url, path), json=_chain(chain_id), timeout=15)
    assert_status_code(response, 201)
    assert response.json()["id"] == chain_id

    response = api.get(api_url(base_url, "taskchains/list"), timeout=15)
    assert_status_code(response, 200)
    assert path in response.json()

    response = api.get(_taskchain_path(base_url, path), timeout=15)
    assert_status_code(response, 200)
    assert response.json()["id"] == chain_id

    updated = _chain(chain_id, description="updated by apitest")
    updated["tasks"].append(
        {
            "id": "finish",
            "handler": "noop",
            "transition": {
                "branches": [{"operator": "default", "when": "", "goto": "end"}],
            },
        }
    )
    response = api.put(_taskchain_path(base_url, path), json=updated, timeout=15)
    assert_status_code(response, 200)
    assert len(response.json()["tasks"]) == 2

    response = api.delete(_taskchain_path(base_url, path), timeout=15)
    assert_status_code(response, 200)

    response = api.get(_taskchain_path(base_url, path), timeout=15)
    assert_status_code(response, 404)


def test_taskchain_path_is_required(api, base_url):
    response = api.get(api_url(base_url, "taskchains"), timeout=15)
    assert_status_code(response, 400)

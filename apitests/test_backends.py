from helpers import api_url, assert_status_code, unique_name


def _backend_payload(name: str) -> dict:
    return {
        "name": name,
        "baseUrl": f"/tmp/contenox-apitest-models/{name}",
        "type": "local",
    }


def test_backend_crud(api, base_url):
    name = unique_name("apitest-backend")
    response = api.post(api_url(base_url, "backends"), json=_backend_payload(name), timeout=15)
    assert_status_code(response, 201)
    backend = response.json()
    backend_id = backend["id"]

    try:
        response = api.get(api_url(base_url, "backends"), timeout=15)
        assert_status_code(response, 200)
        assert any(item["id"] == backend_id for item in response.json())

        response = api.get(api_url(base_url, "backends", backend_id), timeout=15)
        assert_status_code(response, 200)
        details = response.json()
        assert details["name"] == name
        assert details["type"] == "local"
        assert "models" in details
        assert "pulledModels" in details

        updated = _backend_payload(f"{name}-updated")
        response = api.put(api_url(base_url, "backends", backend_id), json=updated, timeout=15)
        assert_status_code(response, 200)
        assert response.json()["name"] == updated["name"]
    finally:
        response = api.delete(api_url(base_url, "backends", backend_id), timeout=15)
        assert_status_code(response, 200)


def test_backend_validation_rejects_missing_base_url(api, base_url):
    response = api.post(
        api_url(base_url, "backends"),
        json={"name": unique_name("apitest-backend"), "type": "local"},
        timeout=15,
    )
    assert response.status_code in (400, 422)

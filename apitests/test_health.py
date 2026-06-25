from helpers import assert_status_code


def test_health_endpoint(api, base_url):
    response = api.get(f"{base_url}/health", timeout=10)
    assert_status_code(response, 200)
    assert response.json() == {"status": "ok"}


def test_version_endpoint(api, base_url):
    response = api.get(f"{base_url}/version", timeout=10)
    assert_status_code(response, 200)
    data = response.json()

    assert isinstance(data.get("version"), str) and data["version"]
    assert isinstance(data.get("nodeInstanceID"), str) and data["nodeInstanceID"]
    assert data.get("tenancy") == "local"

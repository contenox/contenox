import requests
from helpers import assert_status_code

def test_health_endpoint(base_url):
    """Tests if the /health endpoint is reachable and returns 200 OK."""
    response = requests.get(f"{base_url}/health")
    assert_status_code(response, 200)
    # The health endpoint should have an empty body
    assert response.text == ""

def test_version_endpoint(base_url):
    """Tests the /version endpoint for correct structure and data."""
    response = requests.get(f"{base_url}/version")
    assert_status_code(response, 200)
    data = response.json()

    # Check for the presence of required keys
    assert "version" in data
    assert "nodeInstanceID" in data
    assert "tenancy" in data

    # Check that the values are non-empty strings
    assert isinstance(data["version"], str) and data["version"]
    assert isinstance(data["nodeInstanceID"], str) and data["nodeInstanceID"]
    assert isinstance(data["tenancy"], str) and data["tenancy"]

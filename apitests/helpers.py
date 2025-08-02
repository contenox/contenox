def assert_status_code(response, expected_status):
    if response.status_code != expected_status:
        print("\nResponse body on failure:")
        print(response.text)
    assert response.status_code == expected_status

def get_auth_headers(token):
    """Return the authorization header for a given token."""
    return {"Authorization": f"Bearer {token}"}

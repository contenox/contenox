import uuid
from datetime import datetime, timezone, timedelta

def assert_status_code(response, expected_status):
    if response.status_code != expected_status:
        print("\nResponse body on failure:")
        print(response.text)
    assert response.status_code == expected_status

def get_auth_headers(token):
    """Return the authorization header for a given token."""
    return {"Authorization": f"Bearer {token}"}

def generate_unique_name(prefix: str) -> str:
    """Generate a unique name with the given prefix."""
    return f"{prefix}-{str(uuid.uuid4())[:8]}"


def generate_test_event_payload():
    return {
        "id": str(uuid.uuid4()),
        "type": "user.created",
        "source": "auth-service",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "data": {
            "user_id": str(uuid.uuid4()),
            "email": "test@example.com",
            "name": "Test User"
        },
        "metadata": {
            "ip": "192.168.1.1",
            "user_agent": "test-client/1.0"
        }
    }

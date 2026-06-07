import os

import pytest
import requests


BASE_URL = os.environ.get("CONTENOX_API_URL", "http://127.0.0.1:32123/api").rstrip("/")
API_TOKEN = os.environ.get("CONTENOX_API_TOKEN", "").strip()


@pytest.fixture(scope="session")
def base_url() -> str:
    return BASE_URL


@pytest.fixture()
def api() -> requests.Session:
    session = requests.Session()
    if API_TOKEN:
        session.headers.update({"Authorization": f"Bearer {API_TOKEN}"})
    return session

from __future__ import annotations

import uuid
from urllib.parse import urlencode


def assert_status_code(response, expected: int) -> None:
    assert response.status_code == expected, (
        f"expected HTTP {expected}, got {response.status_code}: {response.text}"
    )


def assert_status_in(response, expected: tuple[int, ...]) -> None:
    assert response.status_code in expected, (
        f"expected HTTP status in {expected}, got {response.status_code}: {response.text}"
    )


def unique_name(prefix: str) -> str:
    return f"{prefix}-{uuid.uuid4().hex[:10]}"


def api_url(base_url: str, *parts: str, **query: str) -> str:
    root = base_url.rstrip("/")
    path = "/".join(str(part).strip("/") for part in parts if str(part).strip("/"))
    url = f"{root}/{path}" if path else root
    if query:
        url = f"{url}?{urlencode(query)}"
    return url

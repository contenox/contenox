import os
import uuid

import pytest

from helpers import api_url, assert_status_code, assert_status_in, unique_name


def _registry_url(base_url: str, *parts: str) -> str:
    return api_url(base_url, "model-registry", *parts)


def _entry(name: str | None = None) -> dict:
    model_name = name or unique_name("apitest-model")
    return {
        "name": model_name,
        "sourceUrl": f"https://example.invalid/models/{model_name}.gguf",
        "sizeBytes": 1024,
    }


def test_model_registry_list_and_crud(api, base_url):
    response = api.get(_registry_url(base_url), timeout=15)
    assert_status_code(response, 200)
    assert isinstance(response.json(), list)

    response = api.post(_registry_url(base_url), json=_entry(), timeout=15)
    assert_status_code(response, 201)
    entry = response.json()
    entry_id = entry["id"]

    try:
        response = api.get(_registry_url(base_url, entry_id), timeout=15)
        assert_status_code(response, 200)
        assert response.json()["name"] == entry["name"]

        updated = {**entry, "sourceUrl": "https://example.invalid/updated.gguf", "sizeBytes": 2048}
        response = api.put(_registry_url(base_url, entry_id), json=updated, timeout=15)
        assert_status_code(response, 200)
        assert response.json()["sizeBytes"] == 2048
    finally:
        response = api.delete(_registry_url(base_url, entry_id), timeout=15)
        assert_status_code(response, 200)

    response = api.get(_registry_url(base_url, entry_id), timeout=15)
    assert_status_code(response, 404)


def test_model_registry_create_requires_source_url(api, base_url):
    response = api.post(_registry_url(base_url), json={"name": unique_name("apitest-model")}, timeout=15)
    assert_status_in(response, (400, 422))


def test_model_registry_download_validation(api, base_url):
    response = api.post(_registry_url(base_url, "download"), json={"name": ""}, timeout=15)
    assert_status_in(response, (400, 422))

    response = api.post(
        _registry_url(base_url, "download"),
        json={"name": f"definitely-missing-{uuid.uuid4().hex[:8]}"},
        timeout=15,
    )
    assert_status_in(response, (400, 404, 422))


@pytest.mark.skipif(
    os.environ.get("APITEST_RUN_DOWNLOAD", "").strip() != "1",
    reason="Set APITEST_RUN_DOWNLOAD=1 to run real model download smoke tests.",
)
def test_model_registry_download_curated_model(api, base_url):
    model_name = os.environ.get("APITEST_DOWNLOAD_MODEL", "tiny")
    response = api.post(_registry_url(base_url, "download"), json={"name": model_name}, timeout=600)
    assert_status_code(response, 200)
    assert response.json() in ("downloaded", "already downloaded")

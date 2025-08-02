import pytest
import uuid
import requests
import logging
import time


BASE_URL = "http://localhost:8081"
OLLAMA_URL = "http://host.docker.internal:11435"
DEFAULT_POLL_INTERVAL = 3
DEFAULT_TIMEOUT = 420

# Configure the root logger
logging.basicConfig(level=logging.ERROR,
                    format='%(asctime)s - %(levelname)s - %(name)s - %(message)s')

# Create a logger object for your test module
logger = logging.getLogger(__name__)

@pytest.fixture(scope="session")
def base_url():
    logger.debug("Providing base URL: %s", BASE_URL)
    return BASE_URL

@pytest.fixture(scope="session")
def with_ollama_backend():
    """Check if the Ollama backend is reachable and return its base URL."""
    try:
        location = OLLAMA_URL.replace("host.docker.internal", "localhost")
        response = requests.get(f"{location}", timeout=5)
        response.raise_for_status()
        logger.info("Ollama backend is reachable.")
    except requests.RequestException as e:
        logger.error("Ollama backend not reachable: %s", e)
        pytest.fail(f"Ollama backend check failed: {e}")
    return OLLAMA_URL


@pytest.fixture(scope="session")
def create_backend_and_assign_to_pool(base_url, with_ollama_backend):
    """
    Fixture that creates a backend and assigns it to the 'internal_embed_pool'.
    Returns: dict containing backend_id and pool_id
    """
    ollama_url = with_ollama_backend

    payload = {
        "name": "Test Embedder Backend",
        "baseUrl": ollama_url,
        "type": "ollama",
    }
    response = requests.post(f"{base_url}/backends", json=payload)
    response.raise_for_status()
    backend = response.json()
    backend_id = backend["id"]

    pool_id = "internal_embed_pool"
    assign_url = f"{base_url}/backend-associations/{pool_id}/backends/{backend_id}"
    response = requests.post(assign_url)
    response.raise_for_status()
    assert response.json() == "backend assigned"

    logger.info("Backend %s assigned to pool %s", backend_id, pool_id)

    yield {
        "backend_id": backend_id,
        "pool_id": pool_id,
        "backend": backend,
    }

@pytest.fixture(scope="session")
def create_model_and_assign_to_pool(base_url):
    """
    Fixture that creates a model 'smollm2:135m' and assigns it to the Embedder pool.
    Returns: dict containing model_id and pool_id
    """
    model_name = "smollm2:135m"
    pool_id = "internal_embed_pool"

    payload = {"model": model_name}
    create_url = f"{base_url}/models"
    response = requests.post(create_url, json=payload)
    assert response.status_code == 201, f"Model creation failed: {response.text}"
    model = response.json()
    model_id = model["id"]
    logger.info("Created model: %s", model_id)

    assign_url = f"{base_url}/model-associations/{pool_id}/models/{model_id}"
    response = requests.post(assign_url)
    assert response.status_code == 200, f"Failed to assign model to pool: {response.text}"
    assert response.json() == "model assigned", "Unexpected response when assigning model"
    logger.info("Assigned model %s to pool %s", model_id, pool_id)

    yield {
        "model_id": model_id,
        "model_name": model_name,
        "pool_id": pool_id,
    }

@pytest.fixture(scope="session")
def wait_for_model_in_backend(base_url):
    """
    Fixture that waits until a specific model appears in a backend's pulledModels list.

    Usage:
        def test_something(wait_for_model_in_backend):
            backend_data = wait_for_model_in_backend(model_name="smollm2:135m", backend_id="...")
            assert any(m['name'] == "smollm2:135m" for m in backend_data["pulledModels"])
    """
    def _wait_for_model(*, model_name, backend_id, timeout=DEFAULT_TIMEOUT, poll_interval=DEFAULT_POLL_INTERVAL):
        url = f"{base_url}/backends/{backend_id}"
        start_time = time.time()

        while True:
            try:
                response = requests.get(url)
                if response.status_code != 200:
                    logger.warning("Failed to fetch backend info: %s", response.text)
                    time.sleep(poll_interval)
                    continue

                data = response.json()
                pulled_models = data.get("pulledModels", [])
                logger.debug("Pulled models: %s", [m.get('name') for m in pulled_models])

                if any(m.get('name') == model_name for m in pulled_models):
                    logger.info("✅ Model '%s' found in backend '%s'", model_name, backend_id)
                    return data

                if time.time() - start_time > timeout:
                    raise TimeoutError(
                        f"⏰ Timed out waiting for model '{model_name}' to appear in backend '{backend_id}'. "
                        f"Last response: {data}"
                    )

                logger.info("⏳ Waiting for model '%s' to appear in backend '%s'...", model_name, backend_id)
                time.sleep(poll_interval)

            except requests.RequestException as e:
                logger.warning("Network error while polling backend: %s", str(e))
                time.sleep(poll_interval)

    return _wait_for_model

@pytest.fixture
def create_test_chain(base_url):
    """Fixture to create test chain and clean up after"""
    payload = {
        "id": "test-chain-" + str(uuid.uuid4())[:8],
        "tasks": [{"id": "task1", "type": "llm"}]
    }

    response = requests.post(f"{base_url}/chains",
                           json=payload)
    assert response.status_code == 201

    yield response.json()

    # Teardown
    requests.delete(
        f"{base_url}/chains/{payload['id']}")

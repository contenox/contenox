import pytest
import uuid
import requests
import logging
import time
from pytest_httpserver import HTTPServer
from typing import Generator, Any
import json
from werkzeug.wrappers import Response


BASE_URL = "http://localhost:8081"
OLLAMA_URL = "http://host.docker.internal:11435"
DEFAULT_POLL_INTERVAL = 3
DEFAULT_TIMEOUT = 420

# Configure the root logger
logging.basicConfig(level=logging.INFO,
                    format='%(asctime)s - %(levelname)s - %(name)s - %(message)s')

# Create a logger object for your test module
logger = logging.getLogger(__name__)

def check_function_call_request(request):
    """Asserts the request body is a valid OpenAI-style FunctionCall."""
    try:
        body = request.get_json()
        assert "name" in body, "Request body missing 'name'"
        assert "arguments" in body, "Request body missing 'arguments'"

        # Arguments should be a string containing valid JSON
        try:
            json.loads(body["arguments"])
        except (json.JSONDecodeError, TypeError):
            pytest.fail("'arguments' field is not a valid JSON string")

        return True
    except Exception as e:
        pytest.fail(f"Request body is not a valid FunctionCall: {e}")


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
def create_backend_and_assign_to_group(base_url, with_ollama_backend):
    """
    Fixture that creates a backend and assigns it to the 'internal_embed_group'.
    Returns: dict containing backend_id and group_id
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
    backend_url = backend["baseUrl"]
    group_id = "internal_embed_group"
    assign_url = f"{base_url}/backend-affinity/{group_id}/backends/{backend_id}"
    response = requests.post(assign_url)
    response.raise_for_status()
    assert response.json() == "backend assigned"

    logger.info("Backend %s assigned to group %s", backend_id, group_id)

    group_id = "internal_chat_group"
    assign_url = f"{base_url}/backend-affinity/{group_id}/backends/{backend_id}"
    response = requests.post(assign_url)
    response.raise_for_status()
    assert response.json() == "backend assigned"

    logger.info("Backend %s assigned to group %s", backend_id, group_id)

    group_id = "internal_tasks_group"
    assign_url = f"{base_url}/backend-affinity/{group_id}/backends/{backend_id}"
    response = requests.post(assign_url)
    response.raise_for_status()
    assert response.json() == "backend assigned"

    logger.info("Backend %s assigned to group %s", backend_id, group_id)

    yield {
        "backend_id": backend_id,
        "backend_url": backend_url,
        "group_id": group_id,
        "backend": backend,
    }

@pytest.fixture(scope="session")
def create_model_and_assign_to_group(base_url):
    """
    Fixture that creates a model 'smollm2:135m' and assigns it to the Embedder group.
    Returns: dict containing model_id and group_id
    """
    model_name = "smollm2:135m"
    group_id = "internal_tasks_group"

    payload = {"model": model_name, "canChat": True, "canPrompt": True, "contextLength": 1024}
    create_url = f"{base_url}/models"
    response = requests.post(create_url, json=payload)
    assert response.status_code == 201, f"Model creation failed: {response.text}"
    model = response.json()
    model_id = model["id"]
    logger.info("Created model: %s", model_id)

    assign_url = f"{base_url}/model-affinity/{group_id}/models/{model_id}"
    response = requests.post(assign_url)
    assert response.status_code == 200, f"Failed to assign model to group: {response.text}"
    assert response.json() == "model assigned", "Unexpected response when assigning model"
    logger.info("Assigned model %s to group %s", model_id, group_id)

    yield {
        "model_id": model_id,
        "model_name": model_name,
        "group_id": group_id,
    }

@pytest.fixture(scope="session")
def wait_for_model_in_backend(base_url):
    """
    Enhanced fixture that waits for model download with error handling and progress tracking
    """
    def _wait_for_model(*, model_name, backend_id, timeout=DEFAULT_TIMEOUT, poll_interval=DEFAULT_POLL_INTERVAL):
        url = f"{base_url}/backends/{backend_id}"
        start_time = time.time()
        last_status = None
        download_started = False

        while True:
            try:
                response = requests.get(url)
                if response.status_code != 200:
                    logger.warning("Failed to fetch backend info: %s", response.text)
                    time.sleep(poll_interval)
                    continue

                data = response.json()
                pulled_models = data.get("pulledModels", [])

                # Check for backend errors
                if data.get("error"):
                    error_msg = data["error"]
                    logger.error("Backend error: %s", error_msg)

                # Check if download has started
                if not download_started and any(m.get('name') == model_name for m in pulled_models):
                    logger.info("✅ Model download started for '%s'", model_name)
                    download_started = True

                # Check for completed download
                model_details = next((m for m in pulled_models if m.get('name') == model_name), None)
                if model_details:
                    # Verify successful download
                    if model_details.get("size") > 0 and not model_details.get("error"):
                        logger.info("✅ Model '%s' fully downloaded to backend '%s'", model_name, backend_id)
                        return data
                    elif model_details.get("error"):
                        pytest.fail(f"Model download failed: {model_details['error']}")

                # Report progress if available
                if model_details and model_details.get("progress"):
                    progress = model_details["progress"]
                    if progress != last_status:
                        logger.info("📥 Download progress: %s", progress)
                        last_status = progress

                # Handle timeout
                elapsed = time.time() - start_time
                if elapsed > timeout:
                    pytest.fail(
                        f"⏰ Timed out waiting for model '{model_name}' in backend '{backend_id}'\n"
                        f"Elapsed: {elapsed:.0f}s | Last backend status: {data}"
                    )

                logger.info("⏳ Waiting for model '%s' in backend '%s'...", model_name, backend_id)
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

@pytest.fixture(scope="session")
def httpserver(request) -> Generator[HTTPServer, Any, Any]:
    """
    Session-scoped httpserver fixture.
    This overrides the default function-scoped httpserver from pytest-httpserver.
    It allows other session-scoped fixtures to configure the server.
    """
    server = HTTPServer(host="0.0.0.0", port=0) # Initialize with 0.0.0.0 and dynamic port
    logger.info(f"Attempting to start HTTPServer on {server.host}:{server.port}...")
    server.start()
    logger.info(f"HTTPServer is running and accessible at: {server.url_for('/')}")
    yield server
    server.stop()
    server.clear()

@pytest.fixture(scope="function")
def mock_hook_server(httpserver: HTTPServer):
    endpoint = "/test-hook-endpoint"
    # Default response is a simple JSON object now
    httpserver.expect_request(endpoint, method="POST").respond_with_json({"status": "ok"})
    full_mock_url = httpserver.url_for(suffix=endpoint)
    logger.info(f"Mock hook server endpoint registered at: {full_mock_url}")
    return {
        "url": full_mock_url,
        "server": httpserver
    }

@pytest.fixture
def configurable_mock_hook_server(httpserver: HTTPServer):
    def _setup_mock(status_code=200, response_json=None, delay_seconds=0, expected_headers=None, request_validator=None):
        if response_json is None:
            response_json = {"status": "default_ok"}

        if expected_headers is None:
            expected_headers = {}

        endpoint = f"/test-hook-endpoint-{uuid.uuid4().hex[:8]}"

        def handler(request):
            if delay_seconds > 0:
                time.sleep(delay_seconds)

            # Validate request format if a validator is provided
            if request_validator:
                try:
                    request_validator(request)
                except Exception as e:
                    return Response(
                        json.dumps({"error": f"Request validation failed: {str(e)}"}),
                        400,
                        {"Content-Type": "application/json"}
                    )

            if expected_headers:
                for header_name, expected_value in expected_headers.items():
                    actual_value = request.headers.get(header_name)
                    if actual_value != expected_value:
                        return Response(
                            json.dumps({
                                "error": f"Header validation failed: {header_name} expected={expected_value}, got={actual_value}"
                            }),
                            400,
                            {"Content-Type": "application/json"}
                        )

            return Response(json.dumps(response_json), status_code, {"Content-Type": "application/json"})

        httpserver.expect_request(endpoint, method="POST").respond_with_handler(handler)
        return {
            "url": httpserver.url_for(endpoint),
            "server": httpserver,
            "expected_headers": expected_headers
        }

    return _setup_mock


API_TOKEN = "my-secret-test-token"

@pytest.fixture(scope="session")
def auth_headers():
    """
    Fixture that provides authentication headers with a constant token.
    """
    headers = {
        "X-API-Key": API_TOKEN
    }
    return headers

EMBED_MODEL_NAME = "nomic-embed-text:latest"
TASK_MODEL_NAME = "phi3:3.8b"

@pytest.fixture(scope="session")
def wait_for_declared_models(
    base_url,
    create_backend_and_assign_to_group,
    create_model_and_assign_to_group,
    wait_for_model_in_backend
):
    """Fixture that waits for all declared models to be downloaded"""
    backend_id = create_backend_and_assign_to_group["backend_id"]

    # Get all models from the internal API endpoint
    response = requests.get(f"{base_url}/models")
    response.raise_for_status()
    all_models = response.json()

    # Extract model names
    model_names = [model["model"] for model in all_models]
    logger.info(f"Found {len(model_names)} models to wait for: {model_names}")
    assert len(model_names) > 0, "No models found"

    # Wait for each model to download
    for model_name in model_names:
        logger.info(f"⏳ Waiting for model '{model_name}' to download")
        wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    logger.info("✅ All models downloaded successfully")

import requests
from helpers import assert_status_code

# Test the execute endpoint
def test_execute_prompt(base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]

    # Wait until model is downloaded and visible in backend
    _ =wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)
    payload = {
        "prompt": "What is the capital of France?"
    }
    response = requests.post(f"{base_url}/execute", json=payload)
    assert_status_code(response, 200)

    data = response.json()
    assert "id" in data, "Response missing ID field"
    assert "response" in data, "Response missing response field"
    assert isinstance(data["id"], str), "ID should be a string"
    assert isinstance(data["response"], str), "Response should be a string"
    assert len(data["response"]) > 0, "Response should not be empty"

# Test error handling
def test_execute_without_prompt(base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]

    # Wait until model is downloaded and visible in backend
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)
    payload = {}
    response = requests.post(f"{base_url}/execute", json=payload)
    assert_status_code(response, 400)
    assert "error" in response.json(), "Error response should contain error field"

import requests
from helpers import assert_status_code


def test_create_model(base_url):
    """Test that an admin user can create a model."""

    payload = {
        "model": "test-model",
        "name": "Test Model",
        "canPrompt": True,
        "contextLength": 2048,
    }

    response = requests.post(f"{base_url}/models", json=payload)
    assert_status_code(response, 201)

    model = response.json()
    assert model["model"] == payload["model"], "Model name does not match"
    assert "id" in model, "Missing model ID"

    # Clean up
    model_id = model["model"]
    delete_url = f"{base_url}/models/{model_id}"
    del_response = requests.delete(delete_url, params={"purge": "true"})
    assert_status_code(del_response, 200)


def test_list_models(base_url):
    response = requests.get(f"{base_url}/models")
    assert_status_code(response, 200)

    data = response.json()
    assert data["object"] == "list", "Expected list object"
    assert isinstance(data["data"], list), "Data should be a list"
    assert len(data["data"]) > 0, "No models found"
    model_ids = [model["id"] for model in data["data"]]
    assert "nomic-embed-text:latest" in model_ids, "nomic-embed-text:latest model not found in the list of model IDs"

def test_delete_model(base_url):
    # Step 1: Create a model
    payload = {"model": "temp-delete-model", "canPrompt": True, "contextLength": 2048}
    response = requests.post(f"{base_url}/models", json=payload)
    assert_status_code(response, 201)
    model = response.json()

    # Step 2: Delete it
    model_id = model["model"]
    delete_url = f"{base_url}/models/{model_id}"
    del_response = requests.delete(delete_url, params={"purge": "true"})
    assert_status_code(del_response, 200)

    # Optional: Verify it's gone
    get_response = requests.get(f"{base_url}/models")
    models = get_response.json()["data"]
    assert not any(m["id"] == model_id for m in models), "Model was not deleted"


def test_delete_immutable_model_fails(base_url):
    immutable_model_name = "nomic-embed-text:latest"  # This depends on config

    delete_url = f"{base_url}/models/{immutable_model_name}"
    response = requests.delete(delete_url, params={"purge": "true"})
    assert_status_code(response, 403)

def test_model_assigned_to_pool(base_url, create_model_and_assign_to_pool):
    data = create_model_and_assign_to_pool
    model_id = data["model_id"]
    pool_id = data["pool_id"]

    # Verify assignment by listing models in the pool
    list_url = f"{base_url}/model-associations/{pool_id}/models"
    response = requests.get(list_url)
    assert response.status_code == 200
    models_in_pool = response.json()

    assert any(m['id'] == model_id for m in models_in_pool), "Model not found in pool"

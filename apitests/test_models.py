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

def test_update_model(base_url):
    """Test that an admin user can update an existing model."""

    # Step 1: Create a model
    create_payload = {
        "model": "test-update-model",
        "contextLength": 2048,
        "canChat": False,
        "canPrompt": True,
        "canEmbed": False,
        "canStream": False,
    }

    create_response = requests.post(f"{base_url}/models", json=create_payload)
    assert_status_code(create_response, 201)
    created_model = create_response.json()

    assert created_model["model"] == create_payload["model"]
    assert created_model["contextLength"] == create_payload["contextLength"]
    assert not created_model["canChat"]
    assert created_model["canPrompt"]
    assert not created_model["canEmbed"]
    assert not created_model["canStream"]

    model_id = created_model["model"]

    # Step 2: Update the model
    update_payload = {
        "model": "test-update-model",  # Should match ID
        "contextLength": 4096,
        "canChat": True,
        "canPrompt": False,
        "canEmbed": True,
        "canStream": True,
    }

    update_url = f"{base_url}/models/{model_id}"
    update_response = requests.put(update_url, json=update_payload)
    assert_status_code(update_response, 200)

    updated_model = update_response.json()

    # Step 3: Verify all fields were updated
    assert updated_model["model"] == update_payload["model"], "Model name should remain unchanged"
    assert updated_model["contextLength"] == update_payload["contextLength"], "Context length should be updated"
    assert updated_model["canChat"] == update_payload["canChat"], "canChat flag should be updated"
    assert updated_model["canPrompt"] == update_payload["canPrompt"], "canPrompt flag should be updated"
    assert updated_model["canEmbed"] == update_payload["canEmbed"], "canEmbed flag should be updated"
    assert updated_model["canStream"] == update_payload["canStream"], "canStream flag should be updated"

    # Ensure timestamps were updated (UpdatedAt should be >= CreatedAt)
    assert updated_model["updatedAt"] >= updated_model["createdAt"], "UpdatedAt should be >= CreatedAt"

    # Optional: Verify update is persistent
    get_url = f"{base_url}/internal/models"
    get_response = requests.get(get_url)
    assert_status_code(get_response, 200)
    models = get_response.json()
    updated_model_in_list = next((m for m in models if m["id"] == model_id), None)
    assert updated_model_in_list is not None, "Updated model not found in list"
    assert updated_model_in_list["contextLength"] == 4096
    assert updated_model_in_list["canChat"] is True

    # Step 4: Clean up
    delete_url = f"{base_url}/models/{model_id}"
    del_response = requests.delete(delete_url, params={"purge": "true"})
    assert_status_code(del_response, 200)

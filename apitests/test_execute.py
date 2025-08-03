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

# Test the tasks endpoint with a simple task chain
def test_execute_taskchain(
    base_url,
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

    # Define a simple task chain
    task_chain = {
        "id": "test-chain",
        "debug": True,
        "description": "Test chain for capital city",
        "tasks": [
            {
                "id": "capital_task",
                "type": "raw_string",
                "prompt_template": "What is the capital of France? Respond ONLY with the city name.",
                "transition": {
                    "branches": [
                        {
                            "operator": "default",
                            "goto": "end"
                        }
                    ]
                }
            }
        ],
        "token_limit": 4096
    }

    payload = {
        "input": "France",
        "inputType": "string",
        "chain": task_chain
    }

    # Send request to execute the task chain
    response = requests.post(f"{base_url}/tasks", json=payload)
    assert_status_code(response, 200)

    data = response.json()

    # Validate response structure
    assert "response" in data, "Response missing response field"
    assert "state" in data, "Response missing state field"
    assert isinstance(data["state"], list), "State should be a list"

    # Validate execution history
    assert len(data["state"]) > 0, "State should have at least one entry"
    first_task = data["state"][0]

    assert first_task["taskID"] == "capital_task", "First task should be capital_task"
    assert first_task["taskType"] == "raw_string", "Task type should be raw_string"
    assert first_task["inputType"] == "string", "Input type should be string"
    assert first_task["outputType"] == "string", "Output type should be string"

    # Validate the final response
    assert isinstance(data["response"], str), "Final response should be a string"
    assert "Paris" in data["response"], "Response should contain Paris"

def test_multi_step_taskchain(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    task_chain = {
        "id": "multi-step-chain",
        "debug": True,
        "description": "Multi-step chain test",
        "tasks": [
            {
                "id": "get_country",
                "type": "raw_string",
                "prompt_template": "What country is Paris the capital of?",
                "transition": {
                    "branches": [{"operator": "default", "goto": "capital_task"}]
                }
            },
            {
                "id": "capital_task",
                "type": "raw_string",
                "prompt_template": "What is the capital of {{.get_country}}?",
                "transition": {
                    "branches": [{"operator": "default", "goto": "format_response"}]
                }
            },
            {
                "id": "format_response",
                "type": "raw_string",
                "prompt_template": "Format this nicely: The capital of {{.get_country}} is {{.capital_task}}",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    response = requests.post(f"{base_url}/tasks", json={"input": "Start", "chain": task_chain, "inputType": "string"})
    assert_status_code(response, 200)

    data = response.json()
    assert len(data["state"]) == 3
    assert "The capital of France is Paris" in data["response"]

def test_conditional_branching(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    task_chain = {
        "id": "conditional-chain",
        "debug": True,
        "description": "Conditional branching test",
        "tasks": [
            {
                "id": "check_france",
                "type": "condition_key",
                "valid_conditions": {"yes": True, "no": False},
                "prompt_template": "Is Paris the capital of France? Answer only 'yes' or 'no'.",
                "transition": {
                    "branches": [
                        {"operator": "equals", "when": "yes", "goto": "correct_response"},
                        {"operator": "default", "goto": "incorrect_response"}
                    ]
                }
            },
            {
                "id": "correct_response",
                "type": "noop",
                "prompt_template": "That's correct!",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            },
            {
                "id": "incorrect_response",
                "type": "noop",
                "prompt_template": "That's incorrect!",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    response = requests.post(f"{base_url}/tasks", json={"input": "Start", "chain": task_chain, "inputType": "string"})
    assert_status_code(response, 200)

    data = response.json()
    assert len(data["state"]) >= 2  # At least 2 tasks executed
    last_task = data["state"][-1]
    assert last_task["taskID"] in ["correct_response", "incorrect_response"]
    assert "correct" in data["response"].lower()

def test_invalid_chain_definition(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    # Chain missing required "tasks" field
    invalid_chain = {
        "id": "invalid-chain",
        "description": "This chain is invalid"
    }

    response = requests.post(f"{base_url}/tasks", json={"input": "test", "chain": invalid_chain, "inputType": "string"})
    assert_status_code(response, 400)
    assert "error" in response.json()

def test_model_execution_task(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    task_chain = {
        "id": "model-execution-chain",
        "debug": True,
        "description": "Model execution test",
        "tasks": [
            {
                "id": "chat_task",
                "type": "model_execution",
                "system_instruction": "You are a helpful assistant",
                "execute_config": {
                                    "model": model_name,
                                    "provider": "ollama"
                                },
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    # Create chat history input
    chat_history = {
        "messages": [
            {"role": "user", "content": "What is the capital of France?"}
        ]
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": chat_history, "chain": task_chain, "inputType": "chat_history"}
    )
    assert_status_code(response, 200)

    data = response.json()
    assert len(data["state"]) == 1
    task = data["state"][0]
    assert task["taskID"] == "chat_task"
    assert task["inputType"] == "chat_history"
    assert task["outputType"] == "chat_history"

    # Find the assistant's message in the response
    assistant_messages = [
        msg for msg in data["response"]["messages"]
        if msg["role"] == "assistant"
    ]
    assert len(assistant_messages) == 1, "Expected exactly one assistant message"
    assistant_message = assistant_messages[0]

    # Check if "Paris" is in the assistant's response
    assert "Paris" in assistant_message["content"], \
        f"Expected 'Paris' in response, got: {assistant_message['content']}"

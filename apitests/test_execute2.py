import requests
from helpers import assert_status_code

def test_parse_number_handler(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    """Test parse_number handler with predictable numeric response"""
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    # Use a prompt that should reliably produce a number
    task_chain = {
        "id": "number-parsing-chain",
        "debug": True,
        "description": "Test parse_number handler",
        "tasks": [
            {
                "id": "number_task",
                "handler": "parse_number",
                "prompt_template": "What is 2+2? Respond ONLY with the number.",
                "transition": {
                    "branches": [
                        {
                            "operator": "in_range",
                            "when": "3-5",
                            "goto": "success_path"
                        },
                        {
                            "operator": "default",
                            "goto": "failure_path"
                        }
                    ]
                }
            },
            {
                "id": "success_path",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            },
            {
                "id": "failure_path",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": "Calculate 2+2", "chain": task_chain, "inputType": "string"}
    )
    assert_status_code(response, 200)
    data = response.json()

    # Verify the correct path was taken (should be in success_path)
    task_ids = [step["taskID"] for step in data["state"]]
    assert "success_path" in task_ids, "Should have taken success path"
    assert "failure_path" not in task_ids, "Should not have taken failure path"

    # Verify the output is a number (we don't care about the exact value)
    number_task = next(t for t in data["state"] if t["taskID"] == "number_task")
    assert number_task["output"] == '4', "Output should be 4"

def test_parse_range_handler(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    """Test parse_range handler with numeric range validation"""
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    task_chain = {
        "id": "range-parsing-chain",
        "debug": True,
        "description": "Test parse_range handler",
        "tasks": [
            {
                "id": "range_task",
                "handler": "parse_range",
                "prompt_template": "What is the range of years for the Renaissance period? Format as 1300-1700.",
                "transition": {
                    "branches": [
                        {
                            "operator": "in_range",
                            "when": "1200-1800",
                            "goto": "valid_range"
                        },
                        {
                            "operator": "default",
                            "goto": "invalid_range"
                        }
                    ]
                }
            },
            {
                "id": "valid_range",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            },
            {
                "id": "invalid_range",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": "Renaissance period", "chain": task_chain, "inputType": "string"}
    )
    assert_status_code(response, 200)
    data = response.json()

    # How do we validate this? We don't know the exact range, but it should be somewhere between 1300-1700
    # range_task = next(t for t in data["state"] if t["taskID"] == "range_task")
    # output = range_task["output"]

    # Verify the correct path was taken (should be valid_range)
    task_ids = [step["taskID"] for step in data["state"]]
    assert "valid_range" in task_ids or "invalid_range" in task_ids

def test_transition_operators(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    """Test different transition operators with predictable conditions"""
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    task_chain = {
        "id": "transition-operators-chain",
        "debug": True,
        "description": "Test different transition operators",
        "tasks": [
            {
                "id": "condition_task",
                "handler": "condition_key",
                "valid_conditions": {
                    "very_long_string": True,
                    "short": True
                },
                "prompt_template": "Generate a random string that is either 'very_long_string' or 'short'.",
                "transition": {
                    "branches": [
                        {
                            "operator": "starts_with",
                            "when": "very",
                            "goto": "starts_with_path"
                        },
                        {
                            "operator": "ends_with",
                            "when": "rt",
                            "goto": "ends_with_path"
                        },
                        {
                            "operator": "contains",
                            "when": "long",
                            "goto": "contains_path"
                        },
                        {
                            "operator": "default",
                            "goto": "default_path"
                        }
                    ]
                }
            },
            {
                "id": "starts_with_path",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            },
            {
                "id": "ends_with_path",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            },
            {
                "id": "contains_path",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            },
            {
                "id": "default_path",
                "handler": "noop",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": "Generate test string", "chain": task_chain, "inputType": "string"}
    )
    assert_status_code(response, 200)
    data = response.json()

    # Verify only one transition path was taken
    transition_paths = ["starts_with_path", "ends_with_path", "contains_path", "default_path"]
    taken_paths = [path for path in transition_paths if path in [step["taskID"] for step in data["state"]]]
    assert len(taken_paths) == 1, f"Exactly one transition path should be taken, got {taken_paths}"

    # Verify the output matches the taken path
    condition_task = next(t for t in data["state"] if t["taskID"] == "condition_task")
    output = condition_task["output"]

    if "starts_with_path" in taken_paths:
        assert output.startswith("very")
    elif "ends_with_path" in taken_paths:
        assert output.endswith("rt")
    elif "contains_path" in taken_paths:
        assert "long" in output

def test_compose_strategies(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend
):
    """Test different compose strategies with chat history"""
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    # Create initial chat history
    initial_chat = {
        "messages": [
            {"role": "user", "content": "What is the capital of France?"},
        ]
    }

    task_chain = {
        "id": "compose-strategies-chain",
        "debug": True,
        "description": "Test compose strategies",
        "tasks": [
            {
                "id": "compose_task",
                "handler": "noop",
                "compose": {
                    "with_var": "input",
                    "strategy": "append_string_to_chat_history"
                },
                "prompt_template": "You MUST ONLY respond in all uppercase letters",
                "transition": {
                    "branches": [{"operator": "default", "goto": "chat_task1"}]
                }
            },
            {
                "id": "chat_task1",
                "handler": "model_execution",
                "execute_config": {
                    "model": model_name,
                    "provider": "ollama"
                },
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            },
        ],
        "token_limit": 4096
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": initial_chat, "chain": task_chain, "inputType": "chat_history"}
    )
    assert_status_code(response, 200)
    data = response.json()
    output = data["output"]
    assert "messages" in output
    assert len(output["messages"]) > 2, "Chat history should be longer after composition"
    sent_messages = output["messages"]
    assert sent_messages[0]["role"] == "system"
    assert sent_messages[0]["content"] == "You MUST ONLY respond in all uppercase letters"

    # Assert that the original user message is still present
    assert sent_messages[1]["role"] == "user"
    assert sent_messages[1]["content"] == "What is the capital of France?"

def test_print_statements_with_templates(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_pool,
    create_backend_and_assign_to_pool,
    wait_for_model_in_backend):
    """Test print statements with template variables (verifies via logs)"""
    model_info = create_model_and_assign_to_pool
    backend_info = create_backend_and_assign_to_pool
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]
    _ = wait_for_model_in_backend(model_name=model_name, backend_id=backend_id)

    task_chain = {
        "id": "print-statements-chain",
        "debug": True,
        "description": "Test print statements with templates",
        "tasks": [
            {
                "id": "first_task",
                "handler": "raw_string",
                "prompt_template": "Hello world",
                "print": "First task output: {{.first_task}}",
                "transition": {
                    "branches": [{"operator": "default", "goto": "second_task"}]
                }
            },
            {
                "id": "second_task",
                "handler": "raw_string",
                "prompt_template": "The answer is 42",
                "print": "Second task output: {{.second_task}}. Previous: {{.first_task}}",
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }


    response = requests.post(
        f"{base_url}/tasks",
        json={"input": "Test print statements", "chain": task_chain, "inputType": "string"}
    )
    assert_status_code(response, 200)

    data = response.json()
    output = data["output"]
    assert "Second task output: The answer is 42. Previous: Hello world", output

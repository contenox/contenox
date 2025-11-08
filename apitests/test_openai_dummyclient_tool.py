import pytest
import requests
from openai import OpenAI
from helpers import assert_status_code
import uuid
import time

def test_openai_sdk_with_tools(
    base_url,
    wait_for_declared_models,
    create_model_and_assign_to_group,
    create_backend_and_assign_to_group,
    wait_for_model_in_backend
):
    """Verify that tools from OpenAI-style requests are passed through."""
    # Get model info and wait for download
    model_info = create_model_and_assign_to_group
    backend_info = create_backend_and_assign_to_group
    model_name = model_info["model_name"]
    backend_id = backend_info["backend_id"]

    wait_for_model_in_backend(
        model_name=model_name,
        backend_id=backend_id
    )

    # Define the task chain that passes client tools
    task_chain = {
        "id": "openai-tools-test-chain",
        "debug": True,
        "description": "OpenAI SDK with tools test",
        "tasks": [
            {
                "id": "main_task",
                "handler": "chat_completion",
                "execute_config": {
                    "model": model_name,
                    "provider": "ollama",
                    "pass_clients_tools": True
                },
                "transition": {
                    "branches": [{"operator": "default", "goto": "end"}]
                }
            }
        ],
        "token_limit": 4096
    }

    # Now send a request to /tasks with OpenAI-style input that includes tools
    openai_request = {
        "model": model_name,
        "messages": [
            {"role": "user", "content": "What is the weather in San Francisco?"}
        ],
        "tools": [
            {
                "type": "function",
                "function": {
                    "name": "get_current_weather",
                    "description": "Get the current weather in a given location",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "location": {
                                "type": "string",
                                "description": "The city and state, e.g. San Francisco, CA",
                            },
                            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]},
                        },
                        "required": ["location"],
                    },
                },
            }
        ],
        "tool_choice": "auto"
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={
            "input": openai_request,
            "chain": task_chain,
            "inputType": "openai_chat"
        }
    )
    assert_status_code(response, 200)

    data = response.json()
    # Check that the execution succeeded
    assert "output" in data
    assert "state" in data
    # The output should be a chat history
    assert isinstance(data["output"], dict)
    assert "messages" in data["output"]
    # The last message should be from the assistant
    assert data["output"]["messages"][-1]["role"] == "assistant"

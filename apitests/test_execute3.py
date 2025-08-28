import requests
from helpers import assert_status_code

def test_model_execution_with_openai_chat_input(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_group,
    create_backend_and_assign_to_group,
    wait_for_model_in_backend
):
    """
    Tests that the model_execution handler can directly accept and process
    an input of type 'openai_chat'.
    """
    model_info = create_model_and_assign_to_group
    backend_info = create_backend_and_assign_to_group
    wait_for_model_in_backend(model_name=model_info["model_name"], backend_id=backend_info["backend_id"])

    task_chain = {
        "id": "openai-direct-exec-chain", "debug": True,
        "tasks": [{"id": "chat_task", "handler": "model_execution", "system_instruction":"You are a task processing engine talking to other machines. Return the direct answer without explanation to the given task.", "transition": {"branches": [{"operator": "default", "goto": "end"}]}}],
    }

    openai_request_payload = {
        "model": model_info["model_name"],
        "messages": [{"role": "user", "content": "What is the capital of Italy? Respond only with the city name."}],
        "temperature": 0.1
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": openai_request_payload, "chain": task_chain, "inputType": "openai_chat"}
    )
    assert_status_code(response, 200)

    data = response.json()
    assert data["state"][0]["outputType"] == "chat_history"
    output_history = data["output"]
    assert len(output_history["messages"]) == 3
    assistant_msg = next(m for m in output_history["messages"] if m["role"] == "assistant")
    assert "Rome" in assistant_msg["content"]

def test_embedding_handler_with_openai_chat_input(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_group,
    create_backend_and_assign_to_group,
    wait_for_model_in_backend
):
    """
    Tests that handlers using 'getPrompt' (like embedding) can now accept
    'openai_chat' input, operating on the last message.
    """
    model_info = create_model_and_assign_to_group
    backend_info = create_backend_and_assign_to_group
    wait_for_model_in_backend(model_name=model_info["model_name"], backend_id=backend_info["backend_id"])

    task_chain = {
        "id": "embedding-with-chat", "debug": True,
        "tasks": [{"id": "embed_task", "handler": "embedding", "transition": {"branches": [{"operator": "default", "goto": "end"}]}}],
    }

    openai_request_payload = {
        "messages": [
            {"role": "system", "content": "Ignore this."},
            {"role": "user", "content": "This is the text to embed."}
        ]
    }

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": openai_request_payload, "chain": task_chain, "inputType": "openai_chat"}
    )
    assert_status_code(response, 200)

    data = response.json()
    embed_task_state = data["state"][0]
    assert embed_task_state["inputType"] == "openai_chat"
    assert embed_task_state["outputType"] == "vector"
    embedding_vector = data["output"]
    assert isinstance(embedding_vector, list) and len(embedding_vector) > 0

def test_convert_to_openai_response_handler(
    base_url,
    with_ollama_backend,
    create_model_and_assign_to_group,
    create_backend_and_assign_to_group,
    wait_for_model_in_backend
):
    """
    Tests the new 'convert_to_openai_chat_response' handler.
    """
    model_info = create_model_and_assign_to_group
    backend_info = create_backend_and_assign_to_group
    wait_for_model_in_backend(model_name=model_info["model_name"], backend_id=backend_info["backend_id"])

    task_chain = {
        "id": "formatter-chain", "debug": True,
        "tasks": [
            {
                "id": "chat_task",
                "handler": "model_execution",
                "execute_config": {
                    "model": model_info["model_name"],
                    "provider": "ollama"
                },
                "transition": {"branches": [{"operator": "default", "goto": "format_output"}]}
            },
            {
                "id": "format_output",
                "handler": "convert_to_openai_chat_response",
                "transition": {"branches": [{"operator": "default", "goto": "end"}]}
            }
        ],
    }

    chat_history = {"messages": [{"role": "user", "content": "What is 1+1?"}]}

    response = requests.post(
        f"{base_url}/tasks",
        json={"input": chat_history, "chain": task_chain, "inputType": "chat_history"}
    )
    assert_status_code(response, 200)

    data = response.json()
    final_output = data["output"]

    assert "id" in final_output and final_output["object"] == "chat.completion"
    assert "choices" in final_output and len(final_output["choices"]) == 1
    assert final_output["choices"][0]["message"]["role"] == "assistant"

import json
import os
from pathlib import Path

import ollama
import openai
import pytest
import requests


@pytest.fixture(scope="session")
def gateway():
    url = os.environ.get("CONTENOX_GATEWAY_BASE_URL", "").rstrip("/")
    if not url:
        pytest.skip("gateway API tests require CONTENOX_GATEWAY_BASE_URL")
    token = Path(os.environ["CONTENOX_GATEWAY_TOKEN_FILE"]).read_text().strip()
    limited_token = Path(os.environ["CONTENOX_GATEWAY_LIMITED_TOKEN_FILE"]).read_text().strip()
    model = os.environ["CONTENOX_GATEWAY_TEST_MODEL"]
    tool_model = os.environ["CONTENOX_GATEWAY_TOOL_MODEL"]
    with openai.OpenAI(base_url=url + "/v1", api_key=token, max_retries=0, timeout=20) as client:
        yield {"url": url, "token": token, "limited_token": limited_token,
               "openai": client,
               "ollama": ollama.Client(host=url, headers={"Authorization": "Bearer " + token}, timeout=20),
               "model": model, "tool_model": tool_model}


@pytest.fixture
def messages():
    return [{"role": "user", "content": "hello"}]


def test_catalog_and_chat_with_both_sdks(gateway, messages):
    client = gateway["openai"]
    assert gateway["model"] in [m.id for m in client.models.list().data]
    response = client.chat.completions.create(model=gateway["model"], messages=messages)
    assert response.object == "chat.completion"
    assert response.choices[0].message.content == "hello from gateway"
    assert response.choices[0].finish_reason == "stop"
    assert response.usage.prompt_tokens == 12
    assert response.usage.completion_tokens == 4
    assert response.usage.total_tokens == 16
    other = gateway["ollama"]
    assert gateway["model"] in [m.model for m in other.list().models]
    assert other.show(gateway["model"]).modelinfo
    reply = other.chat(model=gateway["model"], messages=messages, stream=False)
    assert reply.message.content == "hello from gateway"
    assert reply.eval_count == 4


def test_streaming_with_both_sdks(gateway, messages):
    frames = list(gateway["openai"].chat.completions.create(
        model=gateway["model"], messages=messages, stream=True, stream_options={"include_usage": True}))
    assert len({frame.id for frame in frames}) == 1
    assert frames[0].choices[0].delta.role == "assistant"
    assert "".join(f.choices[0].delta.content or "" for f in frames if f.choices) == "hello from gateway"
    assert frames[-2].choices[0].finish_reason == "stop"
    assert frames[-1].choices == []
    assert frames[-1].usage.total_tokens == 16
    ollama_frames = list(gateway["ollama"].chat(model=gateway["model"], messages=messages, stream=True))
    assert "".join(f.message.content or "" for f in ollama_frames) == "hello from gateway"
    assert ollama_frames[-1].done


def test_embeddings_with_sdk_default_base64_and_float(gateway):
    client = gateway["openai"]
    default = client.embeddings.create(model=gateway["model"], input=["one", "two"])
    floats = client.embeddings.create(model=gateway["model"], input="one", encoding_format="float")
    other = gateway["ollama"].embed(model=gateway["model"], input=["one", "two"])
    assert [d.index for d in default.data] == [0, 1]
    assert len(default.data[0].embedding) == 8
    assert default.data[0].embedding == pytest.approx(floats.data[0].embedding)
    assert default.data[1].embedding == pytest.approx(other.embeddings[1])


def test_auth_and_allowlist(gateway, messages):
    with openai.OpenAI(base_url=gateway["url"] + "/v1", api_key="invalid", max_retries=0) as client:
        with pytest.raises(openai.AuthenticationError):
            client.models.list()
    with pytest.raises(openai.PermissionDeniedError):
        gateway["openai"].chat.completions.create(model="not-allowed", messages=messages)


@pytest.mark.parametrize("options", [{"n": 2}, {"stop": "end"},
    {"response_format": {"type": "json_object"}}, {"tool_choice": "required"}, {"logprobs": True}])
def test_unsupported_contracts_fail_explicitly(gateway, messages, options):
    with pytest.raises(openai.BadRequestError):
        gateway["openai"].chat.completions.create(model=gateway["model"], messages=messages, **options)


def test_shared_allowances_and_usage(gateway, messages):
    limited = gateway["limited_token"]
    with openai.OpenAI(base_url=gateway["url"] + "/v1", api_key=limited, max_retries=0) as client:
        client.chat.completions.create(model=gateway["model"], messages=messages)
        with pytest.raises(openai.RateLimitError):
            client.chat.completions.create(model=gateway["model"], messages=messages)
    other = ollama.Client(host=gateway["url"], headers={"Authorization": "Bearer " + limited})
    with pytest.raises(ollama.ResponseError) as error:
        other.chat(model=gateway["model"], messages=messages)
    assert error.value.status_code == 429


@pytest.mark.parametrize("stream", [False, True])
def test_tool_calls_reach_native_openai_client(gateway, messages, stream):
    tools = [{"type": "function", "function": {"name": "weather", "parameters": {
        "type": "object", "properties": {"city": {"type": "string"}}}}}]
    result = gateway["openai"].chat.completions.create(model=gateway["tool_model"], messages=messages, tools=tools, stream=stream)
    if not stream:
        assert result.choices[0].finish_reason == "tool_calls"
        call = result.choices[0].message.tool_calls[0]
        assert call.id == "call_weather"
        assert call.function.name == "weather"
        assert json.loads(call.function.arguments) == {"city": "Berlin"}
    else:
        frames = list(result)
        calls = [call for frame in frames for choice in frame.choices for call in choice.delta.tool_calls or []]
        assert calls[0].index == 0
        assert calls[0].id == "call_weather"
        assert json.loads("".join(c.function.arguments or "" for c in calls)) == {"city": "Berlin"}
        assert frames[-1].choices[0].finish_reason == "tool_calls"


def test_remote_images_are_not_fetched(gateway):
    response = requests.post(gateway["url"] + "/v1/chat/completions",
                             headers={"Authorization": "Bearer " + gateway["token"]},
                             json={"model": gateway["model"], "messages": [{"role": "user", "content": [
                                 {"type": "image_url", "image_url": {"url": "http://127.0.0.1/private"}}]}]}, timeout=10)
    assert response.status_code == 400
    assert "remote URLs" in response.json()["error"]["message"]

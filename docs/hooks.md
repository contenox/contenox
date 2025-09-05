# Hooks: External System Integration

## What Are Hooks?

Hooks are custom integrations that enable workflows to call external systems. They bridge the gap between the task engine and other services, allowing you to build powerful, context-aware automations.

### Primary Use Cases

Hooks are versatile and can be used for two main purposes:

1.  **Assembling Context:** This is a primary use case. Hooks can fetch data from external APIs, databases, or other services. The JSON object returned by the hook can then be passed to subsequent tasks, providing rich, real-time information for an LLM or other processing steps.

2.  **Performing Actions:** Hooks can trigger side-effects in external systems. This includes sending notifications (e.g., to Slack or via email), updating records in a CRM, or initiating other business processes.

-----

## Create a Simple Ping Pong Hook

This example demonstrates how to create a simple external web service and integrate it as a hook.

### 1\. Implement the Hook Service

Create a Python file `ping_pong.py`. The service expects an LangChain, Ollama or OpenAI-style `FunctionCall` object and returns a direct JSON object as its output.

```python
from flask import Flask, request, jsonify
import json

app = Flask(__name__)

@app.route('/ping-pong', methods=['POST'])
def ping_pong():
    # The request body is an OpenAI-style FunctionCall
    function_call_data = request.json

    # The actual parameters are in the 'arguments' field as a JSON string
    try:
        arguments_str = function_call_data.get('arguments', '{}')
        arguments = json.loads(arguments_str)
    except json.JSONDecodeError:
        return jsonify({"error": "Invalid arguments format"}), 400

    input_data = arguments.get('input', '')

    # Simple ping-pong logic
    response_message = "pong" if input_data == "ping" else f"echo: {input_data}"

    # The hook returns its output as a direct JSON object.
    # The task engine will receive this entire object as the task's output.
    return jsonify({
        "message": response_message,
        "received_input": input_data
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
```

Run the service from your terminal:

```bash
python ping_pong.py
```

### 2\. Register the Hook

Register the running service with the `taskengine`:

```bash
curl -X POST http://localhost:8081/hooks/remote \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ping-pong",
    "endpointUrl": "http://host.docker.internal:5000/ping-pong",
    "method": "POST",
    "timeoutMs": 2000,
    "protocolType": "openai"
  }'
```

*Note: We use `host.docker.internal` to allow the task engine container to reach the service running on the host machine.*

### 3\. Use the Hook in a Workflow

Create a task chain that calls the hook. We'll use the new `output_template` field to extract the `message` from the hook's JSON response and use it for a transition.

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "input": "ping",
    "inputType": "string",
    "chain": {
      "id": "ping-pong-demo",
      "tasks": [
        {
          "id": "call_ping_pong",
          "handler": "hook",
          "hook": {
            "name": "ping-pong"
          },
          "output_template": "{{.message}}",
          "transition": {
            "branches": [
              {
                "operator": "equals",
                "when": "pong",
                "goto": "was_a_success"
              },
              {
                "operator": "default",
                "goto": "end"
              }
            ]
          }
        },
        {
          "id": "was_a_success",
          "handler": "noop",
          "transition": { "branches": [{"operator": "default", "goto": "end"}]}
        }
      ]
    }
  }'
```

When this chain runs, the `output_template` transforms the hook's JSON response `{"message": "pong", ...}` into the simple string `"pong"`. This string is then used to evaluate the transition, correctly routing the workflow to the `was_a_success` task.

-----

## Hook Request Format

The system sends a JSON payload in the OpenAI `FunctionCall` format to all remote hook services:

```json
{
  "name": "ping-pong",
  "arguments": "{\"input\":\"ping\",\"param1\":\"value1\"}"
}
```

  - **name**: The name of the hook being called.
  - **arguments**: A **string** containing a JSON object. This object includes the task's `input` and any `args` defined in the task definition. Your service must parse this string to get the parameters.

-----

## Hook Response Format

Hook services must respond with a direct JSON object. This object becomes the output of the hook task and will have the data type `json`.

A valid response can be any JSON object:

```json
{
  "message": "pong",
  "received_input": "ping",
  "processed": true
}
```

There is no longer a required wrapper with `output`, `dataType`, or `transition`.

-----

## Common Errors

  - **`endpoint URL must be absolute`** - Ensure the URL starts with `http://` or `https://`.
  - **`hook failed with status 500`** - Check your remote hook service's logs for errors.
  - **`timeout must be positive`** - The `timeoutMs` value must be greater than zero.
  - **`failed to render hook output template`** - Check that your `output_template` syntax is correct and the fields exist in the hook's JSON response (e.g., `{{.message}}` requires a `message` key in the response).

-----

## Verify Hook Registration

Check all currently registered hooks:

```bash
curl http://localhost:8081/hooks
```

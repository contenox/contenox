# Hooks: External System Integration

## What Are Hooks?

Hooks are remote services that the `taskengine` can call during a task-chain execution. They are the primary mechanism for giving AI agents and workflows access to external tools and APIs.

### Primary Use Cases

  * **Assembling Context:** Fetch data from external APIs, databases, or other services to provide rich, real-time information for an LLM or other processing steps.
  * **Performing Actions:** Trigger side-effects in external systems, such as sending notifications, updating records in a CRM, or initiating other business processes.

-----

## How Hooks Work: The OpenAPI Protocol

Registering a hook that exposes a standard **OpenAPI v3 schema**, the engine can automatically discover all of its API endpoints and make them available as callable tools for an LLM.

  * **Discovery**: The engine fetches the schema from `{endpointUrl}/openapi.json`.
  * **Tool Generation**: It automatically creates namespaced tools (e.g., `my_crm.create_user`) from the API operations.
  * **Execution**: The engine makes standard HTTP requests to the API, mapping LLM arguments to the correct path, query, and body parameters.

-----

## Example: Create a Simple Hook (Direct Function Call)

This example demonstrates how to create a simple web service and integrate it as a direct hook.

### 1\. Implement the Hook Service

Create a Python file `ping_pong.py`. The service expects a request in the `FunctionCall` format and returns a direct JSON object as its output.

```python
from flask import Flask, request, jsonify
import json

app = Flask(__name__)

@app.route('/ping-pong', methods=['POST'])
def ping_pong():
    # The request body is an OpenAI-style FunctionCall
    function_call_data = request.json

    # Parameters are in the 'arguments' field as a JSON string
    try:
        arguments_str = function_call_data.get('arguments', '{}')
        arguments = json.loads(arguments_str)
    except json.JSONDecodeError:
        return jsonify({"error": "Invalid arguments format"}), 400

    input_data = arguments.get('input', '')

    # Simple ping-pong logic
    response_message = "pong" if input_data == "ping" else f"echo: {input_data}"

    # The hook returns its output as a direct JSON object.
    return jsonify({
        "message": response_message,
        "received_input": input_data
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
```

Run the service from the terminal:

```bash
python ping_pong.py
```

### 2\. Register the Hook

Register the running service with the `taskengine`.

```bash
curl -X POST http://localhost:8081/hooks/remote \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ping-pong",
    "endpointUrl": "http://host.docker.internal:5000/ping-pong",
    "timeoutMs": 2000
  }'
```

*Note: `host.docker.internal` allows the runtime container to reach the service running on the host machine.*

### 3\. Use the Hook in a Workflow

Create a task chain that calls the hook. The `output_template` field extracts the `message` from the hook's JSON response to use it for a transition.

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
            "name": "ping-pong",
            "tool_name": "ping_pong"
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

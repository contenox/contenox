# contenox/runtime: Hooks Documentation

## What Are Hooks?

Hooks are **custom integrations** that allow AI workflows to interact with external systems or execute custom logic. They act as bridges between the state machine workflows and the outside world, enabling:

- Call external APIs or services
- Implement custom validation logic
- Integrate with business systems
- Extend the runtime's capabilities

There are **two types of hooks** in contenox/runtime:

1. **Remote Hooks** - HTTP-based services (most common)
2. **Local Hooks** - Built-in Go implementations (for performance-critical operations)

---

## 🌐 Remote Hooks (Recommended Approach)

Remote hooks allow workflows call any HTTP service during execution.

### 1. Register a Remote Hook

Register via API a external service:

```bash
curl -X POST http://localhost:8081/hooks/remote \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ping-pong",
    "endpointUrl": "http://host.docker.internal:5000/ping-pong",
    "method": "POST",
    "timeoutMs": 2000
  }'
```

#### Parameters:

| Parameter | Required | Description | Validation |
|-----------|----------|-------------|------------|
| `name` | Yes | Unique identifier for the hook | Must be unique, non-empty string |
| `endpointUrl` | Yes | Full URL of the service | Must be valid URL |
| `method` | Yes | HTTP method (GET, POST, etc.) | Valid HTTP method |
| `timeoutMs` | Yes | Max execution time in milliseconds | Must be > 0 |

> **Note:** On Linux Docker, use the machine's IP instead of `host.docker.internal`

### 2. Use in a Workflow

Reference the hook in a task-chain workflow:

```json
{
  "input": "ping",
  "inputType": "string",
  "chain": {
    "id": "ping-pong-demo",
    "description": "Simple ping-pong demonstration",
    "tasks": [
      {
        "id": "call_ping_pong",
        "description": "Call our ping-pong hook",
        "handler": "hook",
        "hook": {
          "name": "ping-pong"
        },
        "transition": {
          "branches": [
            {
              "operator": "default",
              "goto": "end"
            }
          ]
        }
      }
    ]
  }
}
```

### 3. Implement a Hook Service

the service must accept a POST request with this JSON structure:

```json
{
  "startingTime": "2023-08-15T12:34:56Z",
  "input": "ping",
  "dataType": "string",
  "transition": "",
  "args": {
    "name": "ping-pong"
  }
}
```

And respond with:

```json
{
  "output": "pong",
  "dataType": "string",
  "error": "",
  "transition": "success"
}
```

#### Response Fields:

| Field | Required | Description |
|-------|----------|-------------|
| `output` | Yes | Result data to pass to next task |
| `dataType` | Yes | Type of the output (string, int, bool, etc.) |
| `error` | No | Error message if any |
| `transition` | Yes | Value used for conditional branching |

### 4. Ping Pong Example (Python)

Minimal working example to test the integration:

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/ping-pong', methods=['POST'])
def ping_pong():
    """Simple hook that responds to 'ping' with 'pong'"""
    data = request.json
    print("Received hook request:", data)

    # Extract the input from the runtime
    input_data = data.get('input', '')

    # Our simple ping-pong logic
    output = "pong" if input_data == "ping" else f"echo: {input_data}"

    # Return the required hook response format
    return jsonify({
        "output": output,
        "dataType": "string",
        "transition": "success"
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
    print("Ping-pong hook server running on http://localhost:5000/ping-pong")
```

**Register it:**
```bash
curl -X POST http://localhost:8081/hooks/remote \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ping-pong",
    "endpointUrl": "http://host.docker.internal:5000/ping-pong",
    "method": "POST",
    "timeoutMs": 2000
  }'
```

**Test it:**
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
          }
        }
      ]
    }
  }'
```

Expected response:
```json
{
  "output": "pong",
  "dataType": "string",
  "state": [
    {
      "taskID": "call_ping_pong",
      "taskHandler": "hook",
      "inputType": "string",
      "outputType": "string",
      "transition": "success",
      "output": "pong"
    }
  ]
}
```

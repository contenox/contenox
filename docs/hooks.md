# contenox/runtime: Hooks Documentation

## What Are Hooks?

Hooks are **custom integrations** enabling AI workflows to interact with external systems or execute custom logic. Hooks act as bridges between state machine workflows and external systems, supporting:

- External API or service calls
- Custom validation logic implementation
- Business system integration
- Runtime capability extension

Two hook types exist in contenox/runtime:

1. **Remote Hooks** - HTTP-based services (most common)
2. **Local Hooks** - Built-in Go implementations (for performance-critical operations)

---

## 🌐 Remote Hooks (Recommended Approach)

Remote hooks allow workflows to call HTTP services during execution.

### 1. Register a Remote Hook

Register an external service via API:

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
| `endpointUrl` | Yes | Full absolute URL of the service | Must include scheme (http:// or https://) and host (e.g., "http://example.com/path") |
| `method` | Yes | HTTP method (GET, POST, etc.) | Valid HTTP method |
| `timeoutMs` | Yes | Max execution time in milliseconds | Must be positive integer (recommended: 1000-30000ms) |

> **Note:** On Linux Docker, use machine IP instead of `host.docker.internal`

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
        "description": "Call ping-pong hook",
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

The service must accept a POST request with this JSON structure:

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
| `dataType` | Yes | Type of the output (must be valid type from table below) |
| `error` | No | Error message if any |
| `transition` | Yes | Value used for conditional branching |

#### Valid dataType Values:
| Value | Description | Example |
|-------|-------------|---------|
| `string` | Text data | `"Hello world"` |
| `int` | Integer number | `42` |
| `float` | Floating point number | `3.14` |
| `bool` | Boolean value | `true` |
| `chat_history` | Chat history object | `{ "messages": [...] }` |
| `openai_chat` | OpenAI chat request | `{ "model": "...", "messages": [...] }` |
| `openai_chat_response` | OpenAI chat response | `{ "choices": [...] }` |
| `search_results` | Search results array | `[ { "title": "...", "content": "..." } ]` |
| `vector` | Vector of floats | `[0.1, 0.2, 0.3]` |
| `json` | Generic JSON object | `{ "any": "valid json" }` |
| `any` | Unspecified type | (not recommended) |

> **Important:** The `output` value undergoes automatic conversion to the type specified in `dataType`. Conversion failures cause hook execution failure. Ensure service output matches declared dataType.

### 4. Error Handling

When remote hooks fail, the system returns specific error messages:

1. **Invalid URL Format**:
   ```
   endpoint URL must be absolute (include http:// or https://): /hooks/legacy/echo
   ```

2. **HTTP Error Status (4xx/5xx)**:
   ```
   hook 'ping-pong' failed with status 500: {"error": "Database connection failed"}
   ```

3. **Invalid JSON Response**:
   ```
   failed to parse response (status 500): invalid character 'D' looking for beginning of value
   Response: Database connection failed
   ```

4. **Non-empty "error" Field**:
   ```
   hook 'ping-pong' error: Database connection failed
   ```

5. **Type Conversion Failure**:
   ```
   hook 'ping-pong' returned invalid int data: cannot convert string to int
   ```

### 5. Ping Pong Example (Python)

Minimal working example to test integration:

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/ping-pong', methods=['POST'])
def ping_pong():
    """Simple hook that responds to 'ping' with 'pong'"""
    data = request.json
    print("Received hook request:", data)

    # Extract input from runtime
    input_data = data.get('input', '')

    # Ping-pong logic
    output = "pong" if input_data == "ping" else f"echo: {input_data}"

    # Return required hook response format
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

### 🔍 Common Hook Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `endpoint URL must be absolute (include http:// or https://)` | Missing scheme in endpointUrl | Use absolute URL: `http://service:port/path` |
| `invalid data type 'xyz'` | Unrecognized dataType value | Use valid dataType from reference table |
| `failed to convert hook output to [type]` | Output incompatible with dataType | Ensure output matches dataType or use compatible type |
| `hook failed with status [code]` | Remote service returned error | Check service logs; response body included in error |
| `context deadline exceeded` | Hook execution exceeded timeout | Increase timeoutMs value |
| `timeout must be positive` | Invalid timeout value | Set timeoutMs to positive integer |

### ✅ Hook Validation Checklist

Before registering a hook:
1. Endpoint URL starts with `http://` or `https://`
2. Service returns valid JSON with proper dataType
3. timeoutMs set between 1000-30000 (1-30 seconds)
4. Successful test via direct curl before workflow integration

```bash
curl -X POST http://localhost:8081/hooks/test \
  -H "Content-Type: application/json" \
  -d '{
    "name": "your-hook",
    "input": "test",
    "dataType": "string"
  }'
```

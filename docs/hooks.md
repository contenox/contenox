# Hooks: External System Integration

## What Are Hooks?
Hooks are custom integrations that enable workflows to call external systems. Hooks bridge between task engine workflows and external services.

## Create a Simple Ping Pong Hook

### 1. Implement the Hook Service
Create a Python file `ping_pong.py`:

```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/ping-pong', methods=['POST'])
def ping_pong():
    data = request.json
    input_data = data.get('input', '')

    # Simple ping-pong logic
    output = "pong" if input_data == "ping" else f"echo: {input_data}"

    return jsonify({
        "output": output,
        "dataType": "string",
        "transition": "success"
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
```

Run the service:
```bash
python ping_pong.py
```

### 2. Register the Hook
Register the external service:

```bash
curl -X POST http://localhost:8081/hooks/remote \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ping-pong",
    "endpointUrl": "http://localhost:5000/ping-pong",
    "method": "POST",
    "timeoutMs": 2000
  }'
```

### 3. Use the Hook in a Workflow
Create a task chain that uses the hook:

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
  }'
```

## Hook Request Format
The system sends this JSON to hook services:

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

## Hook Response Format
Services must respond with this JSON structure:

```json
{
  "output": "pong",
  "dataType": "string",
  "transition": "success",
  "error": ""
}
```

## Supported Data Types
Use these values for the `dataType` field:
- `string` - Text data
- `int` - Integer number
- `float` - Floating-point number
- `bool` - Boolean value
- `chat_history` - Chat conversation
- `vector` - Embedding vector
- `json` - Generic JSON data

## Common Errors
- `endpoint URL must be absolute` - Add http:// or https:// to URL
- `invalid data type 'xyz'` - Use supported data type from list above
- `hook failed with status 500` - Check hook service logs
- `timeout must be positive` - Set timeoutMs to positive integer

## Verify Hook Registration
Check registered hooks:
```bash
curl http://localhost:8081/hooks
```

Test a hook directly:
```bash
curl -X POST http://localhost:8081/hooks/test \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ping-pong",
    "input": "ping",
    "dataType": "string"
  }'
```

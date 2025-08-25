# contenox/runtime: GenAI Orchestration Runtime

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)

**contenox/runtime** is an open-source runtime for orchestrating generative AI workflows. It treats AI workflows as state machines, enabling:

✅ **Declarative workflow definition**
✅ **Built-in state management**
✅ **Vendor-agnostic execution**
✅ **Multi-backend orchestration**
✅ **Observability with passion**
✅ **Made with Go for intensive load**
✅ **Build agentic capabilities via hooks**
✅ **Drop-in for OpenAI chatcompletion API**

-----

## ⚡ Get Started in 1-3 Minutes

This single command will start all necessary services, configure the backend, and download the initial models.

### Prerequisites

  * Docker and Docker Compose
  * `curl` and `jq`

### Run the Bootstrap Script

```bash
git clone https://github.com/contenox/runtime.git
cd runtime
./scripts/bootstrap.sh nomic-embed-text:latest phi3:3.8b # or any other models
# at least one embedding model and one instruction model is required.
```

Once the script finishes, the environment is fully configured and ready to use.

-----

### Try It Out: Execute a Prompt

After the bootstrap is complete, test the setup by executing a simple prompt:

```bash
curl -X POST http://localhost:8081/execute \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Explain quantum computing in simple terms"}'
```

### Next Steps: Create a Workflow

Save the following as `qa.json`:

```json
{
  "input": "What's the best way to optimize database queries?",
  "inputType": "string",
  "chain": {
    "id": "smart-query-assistant",
    "description": "Handles technical questions",
    "tasks": [
      {
        "id": "generate_response",
        "description": "Generate final answer",
        "handler": "raw_string",
        "systemInstruction": "You're a senior engineer. Provide concise, professional answers to technical questions.",
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

Execute the workflow:

```bash
curl -X POST http://localhost:8081/tasks \
  -H "Content-Type: application/json" \
  -d @qa.json
```

All runtime activity is captured in structured logs:

```bash
docker logs contenox-runtime-kernel
```

-----

## ✨ Key Features

### State Machine Engine

  * **Conditional Branching**: Route execution based on LLM outputs
  * **Built-in Handlers**:
      * `condition_key`: Validate and route responses
      * `parse_number`: Extract numerical values
      * `parse_range`: Handle score ranges
      * `raw_string`: Standard text generation
      * `embedding`: Embedding generation
      * `model_execution`: Model execution on a chat history
      * `hook`: Calls a user-defined hook pointing to an external service
  * **Context Preservation**: Automatic input/output passing between steps
  * **Multi-Model Support**: Define preferred models for each task chain
  * **Retry and Timeout**: Configure task-level retries and timeouts for robust workflows

### Multi-Provider Support

Define preferred model provider and backend resolution policy directly within task chains. This allows for seamless, dynamic orchestration across various LLM providers.

```mermaid
flowchart TB
    subgraph User
        U[User/Client Application]
    end

    subgraph API_Layer["API Layer"]
        EP1["POST /execute<br/>Simple Prompt"]
        EP2["POST /tasks<br/>Task Chain"]
        EP3["POST /embed<br/>Embeddings"]
        EP4["GET /supported<br/>Hook Discovery"]
    end

    subgraph Core_Engine["Core Engine"]
        TS[Task Service]
        TE[Task Engine]
        TExec[Task Executor]

        TE --> TExec
        TExec --> MR[Model Repository]
        TExec --> HR[Hook Registry]
    end

    subgraph Model_Management["Model Management"]
        MR --> Resolver[Model Resolver]
        Resolver --> RS[Runtime State]
    end

    subgraph External_Systems["External Systems"]
        subgraph LLM_Providers["LLM Providers"]
            O[Ollama]
            OP[OpenAI API]
            G[Gemini]
            V[vLLM]
        end

        HS[External Hook Servers]
    end

    %% Data flow
    U --> EP1
    U --> EP2
    U --> EP3
    U --> EP4

    EP1 --> TS
    EP2 --> TE
    EP3 --> MR
    EP4 --> HR

    TS --> MR

    Resolver --> LLM_Providers
    HR --> HS

    LLM_Providers -- LLM Responses --> MR
    HS -- Hook Responses --> HR

    MR -- Model Outputs --> TS
    MR -- Model Outputs --> TExec
    HR -- Hook Outputs --> TExec

    TExec -- Task Results --> TE
    TE -- Final Output --> EP2
    TS -- Simple Response --> EP1
    MR -- Embeddings --> EP3
    HR -- Supported Hooks --> EP4

    EP1 --> U
    EP2 --> U
    EP3 --> U
    EP4 --> U

    style Core_Engine fill:#e1f5fe
    style Model_Management fill:#f3e5f5
    style External_Systems fill:#e8f5e8
    style API_Layer fill:#fff3e0

```

  * **Unified Interface**: Consistent API across providers
  * **Automatic Sync**: Models stay consistent across backends
  * **Pool Management**: Assign backends to specific task types
  * **Backend Resolver**: Distribute requests to backends based on resolution policies

-----

## 🧩 Extensibility

### Custom Hooks

Hooks are external servers that can be called from within task chains when registered. They allow interaction with systems and data outside of the runtime and task chains themselves.
[🔗 See Hook Documentation](https://www.google.com/search?q=./docs/hooks.md)

-----

## 📘 API Documentation

The full API surface is thoroughly documented and defined in the OpenAPI format, making it easy to integrate with other tools. You can find more details here:

  * 🔗 [API Reference Documentation](https://www.google.com/search?q=./docs/api-reference.md)
  * 🔗 [View OpenAPI Spec (YAML)](https://www.google.com/search?q=./docs/openapi.yaml)

The [API-Tests](https://www.google.com/search?q=./apitests) are available for additional context.

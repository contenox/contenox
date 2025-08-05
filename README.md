# contenox/runtime: genAI orchestration runtime

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)

**contenox/runtime** is a genAI orchestration runtime that enables developers to build, manage, and execute complex LLM (Large Language Model) workflows with ease. It provides a unified interface for interacting with multiple AI providers, and creating multi-task prompt engineering pipelines.

## Overview

contenox/runtime is designed as the backbone for AI-powered applications, offering:

- **Multi-provider support** for OpenAI, Gemini, Ollama, and custom backends
- **State-machine as engine** with conditional branching and state management
- **Resource pooling** for efficient allocation of AI resources
- **Extensible hook system** for integrating with external services
- **SDK** for seamless integration into applications

## Key Features

### State-machine as Engine
- Chain multiple LLM operations together with conditional transitions
- 10+ built-in task handlers (`parse_number`, `parse_range`, `condition_key`, etc.)
- Dynamic routing based on LLM outputs
- Support for complex input types (chat history, search results, etc.)

```go
// Example state-machine configuration
chain := &taskengine.ChainDefinition{
    ID: "sentiment-analysis",
    Tasks: []taskengine.ChainTask{
        {
            ID:      "classify",
            Handler: taskengine.HandleConditionKey,
            ValidConditions: map[string]bool{"positive": true, "negative": true, "neutral": true},
            PromptTemplate: "Analyze sentiment of: {{.input}}. Respond with positive, negative, or neutral.",
            Transition: taskengine.TaskTransition{
                Branches: []taskengine.TransitionBranch{
                    {Operator: taskengine.OpEquals, When: "positive", Goto: "positive_response"},
                    {Operator: taskengine.OpEquals, When: "negative", Goto: "negative_response"},
                    {Operator: taskengine.OpDefault, Goto: "neutral_response"},
                },
            },
        },
        // Additional tasks...
    },
}
```

### Resource Management
- **Backend Management**: Connect to multiple AI providers (Ollama, OpenAI, Gemini)
- **Model Management**: Download, store, and manage LLM models
- **Resource Pooling**: Group resources by purpose (inference, embedding, etc.)
- **Backend-Model Associations**: Assign specific models to specific backends

### Extensible Architecture
- **Custom Hooks**: Extend functionality with remote HTTP hooks
- **Provider API**: Configure cloud providers (OpenAI, Gemini) securely
- **Download Queue**: Manage model download operations with progress tracking

### SDK
- Implement the same interfaces as internal services
- Seamlessly replace local service with HTTP client
- Consistent error handling across all services
- Full type compatibility with internal models

## Getting Started

## TODO:

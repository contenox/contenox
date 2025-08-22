---
title: contenox/runtime – LLM Backend Management API v0.0.52-9-g761f19a-dirty
language_tabs:
  - python: Python
language_clients:
  - python: ""
toc_footers: []
includes: []
search: true
highlight_theme: darkula
headingLevel: 2

---

<!-- Generator: Widdershins v4.0.1 -->

<h1 id="contenox-runtime-llm-backend-management-api">contenox/runtime – LLM Backend Management API v0.0.52-9-g761f19a-dirty</h1>

> Scroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

# Authentication

* API Key (X-API-Key)
    - Parameter Name: **X-API-Key**, in: header. 

<h1 id="contenox-runtime-llm-backend-management-api-default">Default</h1>

## listPoolsForBackend

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/backend-associations/{backendID}/pools', headers = headers)

print(r.json())

```

`GET /backend-associations/{backendID}/pools`

Lists all pools that a specific backend belongs to.
Useful for understanding which model sets a backend has access to.

<h3 id="listpoolsforbackend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|backendID|path|string|true|none|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
    "name": "production-chat",
    "purposeType": "Internal Tasks",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="listpoolsforbackend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Pool](#schemaarray_runtimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listBackendsByPool

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/backend-associations/{poolID}/backends', headers = headers)

print(r.json())

```

`GET /backend-associations/{poolID}/backends`

Lists all backends associated with a specific pool.
Returns basic backend information without runtime state.

<h3 id="listbackendsbypool-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|poolID|path|string|true|none|

> Example responses

> 200 Response

```json
[
  {
    "baseUrl": "http://ollama-prod.internal:11434",
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
    "name": "ollama-production",
    "type": "ollama",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="listbackendsbypool-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Backend](#schemaarray_runtimetypes_backend)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## removeBackend

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/backend-associations/{poolID}/backends/{backendID}', headers = headers)

print(r.json())

```

`DELETE /backend-associations/{poolID}/backends/{backendID}`

Removes a backend from a pool.
After removal, the backend will no longer be eligible to process requests for models in this pool.
Requests requiring models from this pool will no longer be routed to this backend.

<h3 id="removebackend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|poolID|path|string|true|none|
|backendID|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removebackend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## assignBackend

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/backend-associations/{poolID}/backends/{backendID}', headers = headers)

print(r.json())

```

`POST /backend-associations/{poolID}/backends/{backendID}`

Associates a backend with a pool.
After assignment, the backend can process requests for all models in the pool.
This enables request routing between the backend and models that share this pool.

<h3 id="assignbackend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|poolID|path|string|true|none|
|backendID|path|string|true|none|

> Example responses

> 201 Response

```json
"string"
```

<h3 id="assignbackend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listBackends

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/backends', headers = headers)

print(r.json())

```

`GET /backends`

Lists all configured backend connections with runtime status.
NOTE: Only backends assigned to at least one pool will be used for request processing.
Backends not assigned to any pool exist in the configuration but are completely ignored by the routing system.

> Example responses

> 200 Response

```json
[
  {
    "baseUrl": "http://localhost:11434",
    "createdAt": "2023-01-01T00:00:00Z",
    "error": "error-message",
    "id": "backend-id",
    "models": [
      "string"
    ],
    "name": "backend-name",
    "pulledModels": [
      {
        "canChat": true,
        "canEmbed": false,
        "canPrompt": true,
        "canStream": true,
        "contextLength": 4096,
        "details": {},
        "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
        "model": "mistral:instruct",
        "modifiedAt": "2023-11-15T14:30:45Z",
        "name": "Mistral 7B Instruct",
        "size": 4709611008
      }
    ],
    "type": "ollama",
    "updatedAt": "2023-01-01T00:00:00Z"
  }
]
```

<h3 id="listbackends-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_backendapi_backendSummary](#schemaarray_backendapi_backendsummary)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## createBackend

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/backends', headers = headers)

print(r.json())

```

`POST /backends`

Creates a new backend connection to an LLM provider.
Backends represent connections to LLM services (e.g., Ollama, OpenAI) that can host models.
Note: Creating a backend will be provisioned on the next synchronization cycle.

> Body parameter

```json
{
  "baseUrl": "http://ollama-prod.internal:11434",
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "name": "ollama-production",
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="createbackend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Backend](#schemaruntimetypes_backend)|true|none|

> Example responses

> 201 Response

```json
{
  "baseUrl": "http://ollama-prod.internal:11434",
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "name": "ollama-production",
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="createbackend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_Backend](#schemaruntimetypes_backend)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## deleteBackend

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/backends/{id}', headers = headers)

print(r.json())

```

`DELETE /backends/{id}`

Removes a backend connection.
This does not deleteBackend models from the remote provider, only removes the connection.
Returns a simple "backend removed" confirmation message on success.

<h3 id="deletebackend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletebackend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## getBackend

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/backends/{id}', headers = headers)

print(r.json())

```

`GET /backends/{id}`

Retrieves complete information for a specific backend

<h3 id="getbackend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "baseUrl": "http://ollama-prod.internal:11434",
  "createdAt": "2023-11-15T14:30:45Z",
  "error": "connection timeout: context deadline exceeded",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
  "name": "ollama-production",
  "pulledModels": [
    {
      "canChat": true,
      "canEmbed": false,
      "canPrompt": true,
      "canStream": true,
      "contextLength": 4096,
      "details": {},
      "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
      "model": "mistral:instruct",
      "modifiedAt": "2023-11-15T14:30:45Z",
      "name": "Mistral 7B Instruct",
      "size": 4709611008
    }
  ],
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="getbackend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[backendapi_backendDetails](#schemabackendapi_backenddetails)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## updateBackend

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/backends/{id}', headers = headers)

print(r.json())

```

`PUT /backends/{id}`

Updates an existing backend configuration.
The ID from the URL path overrides any ID in the request body.
Note: Updating a backend will be provisioned on the next synchronization cycle.

> Body parameter

```json
{
  "baseUrl": "http://ollama-prod.internal:11434",
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "name": "ollama-production",
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updatebackend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Backend](#schemaruntimetypes_backend)|true|none|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "baseUrl": "http://ollama-prod.internal:11434",
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "name": "ollama-production",
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updatebackend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_Backend](#schemaruntimetypes_backend)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## defaultModel

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/defaultmodel', headers = headers)

print(r.json())

```

`GET /defaultmodel`

Returns the default model configured during system initialization.

> Example responses

> 200 Response

```json
{
  "modelName": "mistral:latest"
}
```

<h3 id="defaultmodel-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[execapi_DefaultModelResponse](#schemaexecapi_defaultmodelresponse)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## generateEmbeddings

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/embed', headers = headers)

print(r.json())

```

`POST /embed`

Generates vector embeddings for text.
Uses the system's default embedding model configured at startup.
Requests are routed ONLY to backends that have the default model available in any shared pool.
If pools are enabled, models and backends not assigned to any pool will be completely ignored by the routing system.

> Body parameter

```json
{
  "text": "Hello, world!"
}
```

<h3 id="generateembeddings-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[execapi_EmbedRequest](#schemaexecapi_embedrequest)|true|none|

> Example responses

> 200 Response

```json
{
  "vector": "[0.1, 0.2, 0.3, ...]"
}
```

<h3 id="generateembeddings-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[execapi_EmbedResponse](#schemaexecapi_embedresponse)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## executeSimpleTask

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/execute', headers = headers)

print(r.json())

```

`POST /execute`

Runs the prompt through the default LLM.
This endpoint provides basic chat completion optimized for machine-to-machine (M2M) communication.
Requests are routed ONLY to backends that have the default model available in any shared pool.
If pools are enabled, models and backends not assigned to any pool will be completely ignored by the routing system.

> Body parameter

```json
{
  "model_name": "gpt-3.5-turbo",
  "model_provider": "openai",
  "prompt": "Hello, how are you?"
}
```

<h3 id="executesimpletask-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[execservice_TaskRequest](#schemaexecservice_taskrequest)|true|none|

> Example responses

> default Response

```json
{
  "error": "string"
}
```

<h3 id="executesimpletask-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|None|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<h3 id="executesimpletask-responseschema">Response Schema</h3>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## list

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/state', headers = headers)

print(r.json())

```

`GET /state`

Retrieves the current runtime state of all LLM backends.
Includes connection status, loaded models, and error information.
NOTE: This shows the physical state of backends, but the routing system only considers
backends and models that are assigned to the same pool. Resources not in pools are ignored
for request processing even if they appear in this response.

> Example responses

> 200 Response

```json
[
  {
    "backend": {},
    "error": "connection timeout: context deadline exceeded",
    "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
    "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
    "name": "ollama-production",
    "pulledModels": []
  }
]
```

<h3 id="list-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_statetype_BackendRuntimeState](#schemaarray_statetype_backendruntimestate)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## create

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/hooks/remote', headers = headers)

print(r.json())

```

`POST /hooks/remote`

Creates a new remote hook configuration.
Remote hooks allow task-chains to trigger external HTTP services during execution.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "method": "POST",
  "name": "send-email",
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="create-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|true|none|

> Example responses

> 201 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "method": "POST",
  "name": "send-email",
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="create-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## delete

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/hooks/remote/{id}', headers = headers)

print(r.json())

```

`DELETE /hooks/remote/{id}`

Deletes a remote hook configuration by ID.
Returns a simple "deleted" confirmation message on success.

<h3 id="delete-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="delete-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## get

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/providers/{providerType}/config', headers = headers)

print(r.json())

```

`GET /providers/{providerType}/config`

Retrieves configuration details for a specific external provider.

<h3 id="get-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|providerType|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "APIKey": "string",
  "Type": "string"
}
```

<h3 id="get-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimestate_ProviderConfig](#schemaruntimestate_providerconfig)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## update

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/hooks/remote/{id}', headers = headers)

print(r.json())

```

`PUT /hooks/remote/{id}`

Updates an existing remote hook configuration.
The ID from the URL path overrides any ID in the request body.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "method": "POST",
  "name": "send-email",
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="update-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|true|none|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "method": "POST",
  "name": "send-email",
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="update-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listInternal

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/internal/models', headers = headers)

print(r.json())

```

`GET /internal/models`

Lists all registered models in internal format.
This endpoint returns full model details including timestamps and capabilities.
Intended for administrative and debugging purposes.

> Example responses

> default Response

```json
{
  "error": "string"
}
```

<h3 id="listinternal-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|None|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<h3 id="listinternal-responseschema">Response Schema</h3>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listPoolsForModel

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/model-associations/{modelID}/pools', headers = headers)

print(r.json())

```

`GET /model-associations/{modelID}/pools`

Lists all pools that a specific model belongs to.
Useful for understanding where a model is deployed across the system.

<h3 id="listpoolsformodel-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|modelID|path|string|true|none|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
    "name": "production-chat",
    "purposeType": "Internal Tasks",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="listpoolsformodel-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Pool](#schemaarray_runtimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listModelsByPool

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/model-associations/{poolID}/models', headers = headers)

print(r.json())

```

`GET /model-associations/{poolID}/models`

Lists all models associated with a specific pool.
Returns basic model information without backend-specific details.

<h3 id="listmodelsbypool-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|poolID|path|string|true|none|

> Example responses

> 200 Response

```json
[
  {
    "canChat": true,
    "canEmbed": false,
    "canPrompt": true,
    "canStream": true,
    "contextLength": 8192,
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "m7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a",
    "model": "mistral:instruct",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="listmodelsbypool-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Model](#schemaarray_runtimetypes_model)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## removeModel

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/model-associations/{poolID}/models/{modelID}', headers = headers)

print(r.json())

```

`DELETE /model-associations/{poolID}/models/{modelID}`

Removes a model from a pool.
After removal, requests for this model will no longer be routed to backends in this pool.
This model can still be used with backends in other pools where it remains assigned.

<h3 id="removemodel-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|poolID|path|string|true|none|
|modelID|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removemodel-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## assignModel

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/model-associations/{poolID}/models/{modelID}', headers = headers)

print(r.json())

```

`POST /model-associations/{poolID}/models/{modelID}`

Associates a model with a pool.
After assignment, requests for this model can be routed to any backend in the pool.
This enables request routing between the model and backends that share this pool.

<h3 id="assignmodel-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|poolID|path|string|true|none|
|modelID|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="assignmodel-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listModels

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/models', headers = headers)

print(r.json())

```

`GET /models`

Lists all registered models in OpenAI-compatible format.
Returns models as they would appear in OpenAI's /v1/models endpoint.
NOTE: Only models assigned to at least one pool will be available for request processing.
Models not assigned to any pool exist in the configuration but are completely ignored by the routing system.

> Example responses

> 200 Response

```json
{
  "data": [
    {
      "created": 1717020800,
      "id": "mistral:latest",
      "object": "mistral:latest",
      "owned_by": "system"
    }
  ],
  "object": "list"
}
```

<h3 id="listmodels-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[backendapi_OpenAICompatibleModelList](#schemabackendapi_openaicompatiblemodellist)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## createModel

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/models', headers = headers)

print(r.json())

```

`POST /models`

Declares a new model to the system.
The model must be available in a configured backend or will be queued for download.
IMPORTANT: Models not assigned to any pool will NOT be available for request processing.
If pools are enabled, to make a model available to backends, it must be explicitly added to at least one pool.

> Body parameter

```json
{
  "canChat": true,
  "canEmbed": false,
  "canPrompt": true,
  "canStream": true,
  "contextLength": 8192,
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "m7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a",
  "model": "mistral:instruct",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="createmodel-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Model](#schemaruntimetypes_model)|true|none|

> Example responses

> 201 Response

```json
{
  "canChat": true,
  "canEmbed": false,
  "canPrompt": true,
  "canStream": true,
  "contextLength": 8192,
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "m7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a",
  "model": "mistral:instruct",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="createmodel-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_Model](#schemaruntimetypes_model)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## deleteModel

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/models/{id}', headers = headers)

print(r.json())

```

`DELETE /models/{id}`

Deletes a model from the system registry.
- Does not remove the model from backend storage (requires separate backend operation)
- Accepts 'purge=true' query parameter to also remove related downloads from queue

<h3 id="deletemodel-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletemodel-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## updateModel

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/models/{id}', headers = headers)

print(r.json())

```

`PUT /models/{id}`

Updates an existing model registration.
Only mutable fields (like capabilities and context length) can be updated.
The model ID cannot be changed.
Returns the updated model configuration.

> Body parameter

```json
{
  "canChat": true,
  "canEmbed": false,
  "canPrompt": true,
  "canStream": true,
  "contextLength": 8192,
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "m7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a",
  "model": "mistral:instruct",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updatemodel-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Model](#schemaruntimetypes_model)|true|none|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "canChat": true,
  "canEmbed": false,
  "canPrompt": true,
  "canStream": true,
  "contextLength": 8192,
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "m7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a",
  "model": "mistral:instruct",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updatemodel-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_Model](#schemaruntimetypes_model)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## getPoolByName

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/pool-by-name/{name}', headers = headers)

print(r.json())

```

`GET /pool-by-name/{name}`

Retrieves a pool by its human-readable name.
Useful for configuration where ID might not be known but name is consistent.

<h3 id="getpoolbyname-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
  "name": "production-chat",
  "purposeType": "Internal Tasks",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="getpoolbyname-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_Pool](#schemaruntimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listPoolsByPurpose

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/pool-by-purpose/{purpose}', headers = headers)

print(r.json())

```

`GET /pool-by-purpose/{purpose}`

Lists pools filtered by purpose type with pagination support.
Purpose types categorize pools (e.g., "Internal Embeddings", "Internal Tasks").
Accepts 'cursor' (RFC3339Nano timestamp) and 'limit' parameters for pagination.

<h3 id="listpoolsbypurpose-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|purpose|path|string|true|none|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
    "name": "production-chat",
    "purposeType": "Internal Tasks",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="listpoolsbypurpose-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Pool](#schemaarray_runtimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listPools

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/pools', headers = headers)

print(r.json())

```

`GET /pools`

Lists all resource pools in the system.
Returns basic pool information without associated backends or models.

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
    "name": "production-chat",
    "purposeType": "Internal Tasks",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="listpools-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Pool](#schemaarray_runtimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## createPool

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/pools', headers = headers)

print(r.json())

```

`POST /pools`

Creates a new resource pool for organizing backends and models.
Pool names must be unique within the system.
Pools allow grouping of backends and models for specific operational purposes (e.g., embeddings, tasks).
When pools are configured in the system, request routing ONLY considers resources that share a pool.
- Models not assigned to any pool will NOT be available for execution
- Backends not assigned to any pool will NOT receive models or process requests
- Resources must be explicitly associated with the same pool to work together
This is a fundamental operational requirement - resources outside pools are effectively invisible to the routing system.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
  "name": "production-chat",
  "purposeType": "Internal Tasks",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="createpool-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Pool](#schemaruntimetypes_pool)|true|none|

> Example responses

> 201 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
  "name": "production-chat",
  "purposeType": "Internal Tasks",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="createpool-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_Pool](#schemaruntimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## deletePool

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/pools/{id}', headers = headers)

print(r.json())

```

`DELETE /pools/{id}`

Removes a pool from the system.
This does not deletePool associated backends or models, only the pool relationship.
Returns a simple "deleted" confirmation message on success.

<h3 id="deletepool-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletepool-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## getPool

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/pools/{id}', headers = headers)

print(r.json())

```

`GET /pools/{id}`

Retrieves a specific pool by its unique ID.

<h3 id="getpool-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
  "name": "production-chat",
  "purposeType": "Internal Tasks",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="getpool-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_Pool](#schemaruntimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## updatePool

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/pools/{id}', headers = headers)

print(r.json())

```

`PUT /pools/{id}`

Updates an existing pool configuration.
The ID from the URL path overrides any ID in the request body.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
  "name": "production-chat",
  "purposeType": "Internal Tasks",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updatepool-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Pool](#schemaruntimetypes_pool)|true|none|
|id|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
  "name": "production-chat",
  "purposeType": "Internal Tasks",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updatepool-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_Pool](#schemaruntimetypes_pool)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## listConfigs

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/providers/configs', headers = headers)

print(r.json())

```

`GET /providers/configs`

Lists all configured external providers with pagination support.

> Example responses

> 200 Response

```json
[
  {
    "APIKey": "string",
    "Type": "string"
  }
]
```

<h3 id="listconfigs-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimestate_ProviderConfig](#schemaarray_runtimestate_providerconfig)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## deleteConfig

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/providers/{providerType}/config', headers = headers)

print(r.json())

```

`DELETE /providers/{providerType}/config`

Removes provider configuration from the system.
After deletion, the provider will no longer be available for model execution.

<h3 id="deleteconfig-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|providerType|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deleteconfig-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## getQueue

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/queue', headers = headers)

print(r.json())

```

`GET /queue`

Retrieves the current model download queue state.
Returns a list of models waiting to be downloaded.
Downloading models is only supported for ollama backends.
If pools are enabled, models will only be downloaded to backends
that are associated with at least one pool.

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2021-09-01T00:00:00Z",
    "id": "1234567890",
    "modelJob": {},
    "scheduledFor": 1630483200,
    "taskType": "model_download",
    "validUntil": 1630483200
  }
]
```

<h3 id="getqueue-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_downloadservice_Job](#schemaarray_downloadservice_job)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## cancelDownload

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/queue/cancel', headers = headers)

print(r.json())

```

`DELETE /queue/cancel`

Cancels an in-progress model download.
Accepts either:
- 'url' query parameter to cancel a download on a specific backend
- 'model' query parameter to cancel the model download across all backends
Example: /queue/cancel?url=http://localhost:11434
/queue/cancel?model=mistral:latest

> Example responses

> 200 Response

```json
"string"
```

<h3 id="canceldownload-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## inProgress

> Code samples

```python
import requests
headers = {
  'Accept': 'text/event-stream',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/queue/inProgress', headers = headers)

print(r.json())

```

`GET /queue/inProgress`

Streams real-time download progress via Server-Sent Events (SSE).
Clients should handle 'data' events containing JSON status updates.
Connection remains open until client disconnects or server closes.
Example event format:
event: status
data: {"status":"downloading","digest":"sha256:abc123","total":1000000,"completed":250000,"model":"mistral:latest","baseUrl":"http://localhost:11434"}

> Example responses

> 200 Response

> default Response

```json
{
  "error": "string"
}
```

<h3 id="inprogress-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Server-Sent Events stream|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## removeFromQueue

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/queue/{model}', headers = headers)

print(r.json())

```

`DELETE /queue/{model}`

Removes a model from the download queue.
If a model download is in progress or the download will be cancelled.

<h3 id="removefromqueue-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|model|path|string|true|none|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removefromqueue-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## supported

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/supported', headers = headers)

print(r.json())

```

`GET /supported`

Lists available task-chain hook types.
Returns all registered external action types that can be used in task-chain hooks.

> Example responses

> 200 Response

```json
[
  "string"
]
```

<h3 id="supported-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_string](#schemaarray_string)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## executeTaskChain

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/tasks', headers = headers)

print(r.json())

```

`POST /tasks`

Executes dynamic task-chain workflows.
Task-chains are state-machine workflows (DAGs) with conditional branches,
external hooks, and captured execution state.
Requests are routed ONLY to backends that have the requested model available in any shared pool.
If pools are enabled, models and backends not assigned to any pool will be completely ignored by the routing system.

> Body parameter

```json
{
  "chain": [
    {
      "debug": true,
      "description": "string",
      "id": "string",
      "tasks": [
        {
          "compose": {},
          "description": "Validates user input meets quality requirements",
          "execute_config": {},
          "handler": "condition_key",
          "hook": {},
          "id": "validate_input",
          "input_var": "input",
          "print": "Validation result: {{.validate_input}}",
          "prompt_template": "Is this input valid? {{.input}}",
          "retry_on_failure": 2,
          "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
          "timeout": "30s",
          "transition": {},
          "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
        }
      ],
      "token_limit": 0
    }
  ],
  "input": "What is the capital of France",
  "inputType": "string"
}
```

<h3 id="executetaskchain-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[execapi_taskExecutionRequest](#schemaexecapi_taskexecutionrequest)|true|none|

> Example responses

> 200 Response

```json
{
  "output": "Paris",
  "outputType": "string",
  "state": [
    {
      "duration": 452000000,
      "error": {},
      "input": "This is a test input that needs validation",
      "inputType": "string",
      "output": "valid",
      "outputType": "string",
      "taskHandler": "condition_key",
      "taskID": "validate_input",
      "transition": "valid_input"
    }
  ]
}
```

<h3 id="executetaskchain-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[execapi_taskExecutionResponse](#schemaexecapi_taskexecutionresponse)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

# Schemas

<h2 id="tocS_ErrorResponse">ErrorResponse</h2>
<!-- backwards compatibility -->
<a id="schemaerrorresponse"></a>
<a id="schema_ErrorResponse"></a>
<a id="tocSerrorresponse"></a>
<a id="tocserrorresponse"></a>

```json
{
  "error": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|error|string|true|none|none|

<h2 id="tocS_array_backendapi_backendSummary">array_backendapi_backendSummary</h2>
<!-- backwards compatibility -->
<a id="schemaarray_backendapi_backendsummary"></a>
<a id="schema_array_backendapi_backendSummary"></a>
<a id="tocSarray_backendapi_backendsummary"></a>
<a id="tocsarray_backendapi_backendsummary"></a>

```json
[
  {
    "baseUrl": "http://localhost:11434",
    "createdAt": "2023-01-01T00:00:00Z",
    "error": "error-message",
    "id": "backend-id",
    "models": [
      "string"
    ],
    "name": "backend-name",
    "pulledModels": [
      {
        "canChat": true,
        "canEmbed": false,
        "canPrompt": true,
        "canStream": true,
        "contextLength": 4096,
        "details": {},
        "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
        "model": "mistral:instruct",
        "modifiedAt": "2023-11-15T14:30:45Z",
        "name": "Mistral 7B Instruct",
        "size": 4709611008
      }
    ],
    "type": "ollama",
    "updatedAt": "2023-01-01T00:00:00Z"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[backendapi_backendSummary](#schemabackendapi_backendsummary)]|false|none|none|

<h2 id="tocS_array_downloadservice_Job">array_downloadservice_Job</h2>
<!-- backwards compatibility -->
<a id="schemaarray_downloadservice_job"></a>
<a id="schema_array_downloadservice_Job"></a>
<a id="tocSarray_downloadservice_job"></a>
<a id="tocsarray_downloadservice_job"></a>

```json
[
  {
    "createdAt": "2021-09-01T00:00:00Z",
    "id": "1234567890",
    "modelJob": {},
    "scheduledFor": 1630483200,
    "taskType": "model_download",
    "validUntil": 1630483200
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[downloadservice_Job](#schemadownloadservice_job)]|false|none|none|

<h2 id="tocS_array_runtimestate_ProviderConfig">array_runtimestate_ProviderConfig</h2>
<!-- backwards compatibility -->
<a id="schemaarray_runtimestate_providerconfig"></a>
<a id="schema_array_runtimestate_ProviderConfig"></a>
<a id="tocSarray_runtimestate_providerconfig"></a>
<a id="tocsarray_runtimestate_providerconfig"></a>

```json
[
  {
    "APIKey": "string",
    "Type": "string"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[runtimestate_ProviderConfig](#schemaruntimestate_providerconfig)]|false|none|none|

<h2 id="tocS_array_runtimetypes_Backend">array_runtimetypes_Backend</h2>
<!-- backwards compatibility -->
<a id="schemaarray_runtimetypes_backend"></a>
<a id="schema_array_runtimetypes_Backend"></a>
<a id="tocSarray_runtimetypes_backend"></a>
<a id="tocsarray_runtimetypes_backend"></a>

```json
[
  {
    "baseUrl": "http://ollama-prod.internal:11434",
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
    "name": "ollama-production",
    "type": "ollama",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[runtimetypes_Backend](#schemaruntimetypes_backend)]|false|none|none|

<h2 id="tocS_array_runtimetypes_Model">array_runtimetypes_Model</h2>
<!-- backwards compatibility -->
<a id="schemaarray_runtimetypes_model"></a>
<a id="schema_array_runtimetypes_Model"></a>
<a id="tocSarray_runtimetypes_model"></a>
<a id="tocsarray_runtimetypes_model"></a>

```json
[
  {
    "canChat": true,
    "canEmbed": false,
    "canPrompt": true,
    "canStream": true,
    "contextLength": 8192,
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "m7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a",
    "model": "mistral:instruct",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[runtimetypes_Model](#schemaruntimetypes_model)]|false|none|none|

<h2 id="tocS_array_runtimetypes_Pool">array_runtimetypes_Pool</h2>
<!-- backwards compatibility -->
<a id="schemaarray_runtimetypes_pool"></a>
<a id="schema_array_runtimetypes_Pool"></a>
<a id="tocSarray_runtimetypes_pool"></a>
<a id="tocsarray_runtimetypes_pool"></a>

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
    "name": "production-chat",
    "purposeType": "Internal Tasks",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[runtimetypes_Pool](#schemaruntimetypes_pool)]|false|none|none|

<h2 id="tocS_array_runtimetypes_RemoteHook">array_runtimetypes_RemoteHook</h2>
<!-- backwards compatibility -->
<a id="schemaarray_runtimetypes_remotehook"></a>
<a id="schema_array_runtimetypes_RemoteHook"></a>
<a id="tocSarray_runtimetypes_remotehook"></a>
<a id="tocsarray_runtimetypes_remotehook"></a>

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "endpointUrl": "http://hooks-endpoint:port",
    "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
    "method": "POST",
    "name": "send-email",
    "timeoutMs": 5000,
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)]|false|none|none|

<h2 id="tocS_array_statetype_BackendRuntimeState">array_statetype_BackendRuntimeState</h2>
<!-- backwards compatibility -->
<a id="schemaarray_statetype_backendruntimestate"></a>
<a id="schema_array_statetype_BackendRuntimeState"></a>
<a id="tocSarray_statetype_backendruntimestate"></a>
<a id="tocsarray_statetype_backendruntimestate"></a>

```json
[
  {
    "backend": {},
    "error": "connection timeout: context deadline exceeded",
    "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
    "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
    "name": "ollama-production",
    "pulledModels": []
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[statetype_BackendRuntimeState](#schemastatetype_backendruntimestate)]|false|none|none|

<h2 id="tocS_array_string">array_string</h2>
<!-- backwards compatibility -->
<a id="schemaarray_string"></a>
<a id="schema_array_string"></a>
<a id="tocSarray_string"></a>
<a id="tocsarray_string"></a>

```json
[
  "string"
]

```

### Properties

*None*

<h2 id="tocS_backendapi_OpenAICompatibleModelList">backendapi_OpenAICompatibleModelList</h2>
<!-- backwards compatibility -->
<a id="schemabackendapi_openaicompatiblemodellist"></a>
<a id="schema_backendapi_OpenAICompatibleModelList"></a>
<a id="tocSbackendapi_openaicompatiblemodellist"></a>
<a id="tocsbackendapi_openaicompatiblemodellist"></a>

```json
{
  "data": [
    {
      "created": 1717020800,
      "id": "mistral:latest",
      "object": "mistral:latest",
      "owned_by": "system"
    }
  ],
  "object": "list"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|data|[[backendapi_OpenAIModel](#schemabackendapi_openaimodel)]|true|none|none|
|object|string|true|none|none|

<h2 id="tocS_backendapi_OpenAIModel">backendapi_OpenAIModel</h2>
<!-- backwards compatibility -->
<a id="schemabackendapi_openaimodel"></a>
<a id="schema_backendapi_OpenAIModel"></a>
<a id="tocSbackendapi_openaimodel"></a>
<a id="tocsbackendapi_openaimodel"></a>

```json
{
  "created": 1717020800,
  "id": "mistral:latest",
  "object": "mistral:latest",
  "owned_by": "system"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|created|integer|true|none|none|
|id|string|true|none|none|
|object|string|true|none|none|
|owned_by|string|true|none|none|

<h2 id="tocS_backendapi_backendDetails">backendapi_backendDetails</h2>
<!-- backwards compatibility -->
<a id="schemabackendapi_backenddetails"></a>
<a id="schema_backendapi_backendDetails"></a>
<a id="tocSbackendapi_backenddetails"></a>
<a id="tocsbackendapi_backenddetails"></a>

```json
{
  "baseUrl": "http://ollama-prod.internal:11434",
  "createdAt": "2023-11-15T14:30:45Z",
  "error": "connection timeout: context deadline exceeded",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
  "name": "ollama-production",
  "pulledModels": [
    {
      "canChat": true,
      "canEmbed": false,
      "canPrompt": true,
      "canStream": true,
      "contextLength": 4096,
      "details": {},
      "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
      "model": "mistral:instruct",
      "modifiedAt": "2023-11-15T14:30:45Z",
      "name": "Mistral 7B Instruct",
      "size": 4709611008
    }
  ],
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|baseUrl|string|true|none|none|
|createdAt|string(date-time)|true|none|none|
|error|string|false|none|none|
|id|string|true|none|none|
|models|[string]|true|none|none|
|name|string|true|none|none|
|pulledModels|[[statetype_ModelPullStatus](#schemastatetype_modelpullstatus)]|true|none|none|
|type|string|true|none|none|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_backendapi_backendSummary">backendapi_backendSummary</h2>
<!-- backwards compatibility -->
<a id="schemabackendapi_backendsummary"></a>
<a id="schema_backendapi_backendSummary"></a>
<a id="tocSbackendapi_backendsummary"></a>
<a id="tocsbackendapi_backendsummary"></a>

```json
{
  "baseUrl": "http://localhost:11434",
  "createdAt": "2023-01-01T00:00:00Z",
  "error": "error-message",
  "id": "backend-id",
  "models": [
    "string"
  ],
  "name": "backend-name",
  "pulledModels": [
    {
      "canChat": true,
      "canEmbed": false,
      "canPrompt": true,
      "canStream": true,
      "contextLength": 4096,
      "details": {},
      "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
      "model": "mistral:instruct",
      "modifiedAt": "2023-11-15T14:30:45Z",
      "name": "Mistral 7B Instruct",
      "size": 4709611008
    }
  ],
  "type": "ollama",
  "updatedAt": "2023-01-01T00:00:00Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|baseUrl|string|true|none|none|
|createdAt|string(date-time)|true|none|none|
|error|string|false|none|none|
|id|string|true|none|none|
|models|[string]|true|none|none|
|name|string|true|none|none|
|pulledModels|[[statetype_ModelPullStatus](#schemastatetype_modelpullstatus)]|true|none|none|
|type|string|true|none|none|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_downloadservice_Job">downloadservice_Job</h2>
<!-- backwards compatibility -->
<a id="schemadownloadservice_job"></a>
<a id="schema_downloadservice_Job"></a>
<a id="tocSdownloadservice_job"></a>
<a id="tocsdownloadservice_job"></a>

```json
{
  "createdAt": "2021-09-01T00:00:00Z",
  "id": "1234567890",
  "modelJob": {},
  "scheduledFor": 1630483200,
  "taskType": "model_download",
  "validUntil": 1630483200
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|createdAt|string(date-time)|true|none|none|
|id|string|true|none|none|
|modelJob|object|true|none|none|
|scheduledFor|integer|true|none|none|
|taskType|string|true|none|none|
|validUntil|integer|true|none|none|

<h2 id="tocS_execapi_DefaultModelResponse">execapi_DefaultModelResponse</h2>
<!-- backwards compatibility -->
<a id="schemaexecapi_defaultmodelresponse"></a>
<a id="schema_execapi_DefaultModelResponse"></a>
<a id="tocSexecapi_defaultmodelresponse"></a>
<a id="tocsexecapi_defaultmodelresponse"></a>

```json
{
  "modelName": "mistral:latest"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|modelName|string|true|none|none|

<h2 id="tocS_execapi_EmbedRequest">execapi_EmbedRequest</h2>
<!-- backwards compatibility -->
<a id="schemaexecapi_embedrequest"></a>
<a id="schema_execapi_EmbedRequest"></a>
<a id="tocSexecapi_embedrequest"></a>
<a id="tocsexecapi_embedrequest"></a>

```json
{
  "text": "Hello, world!"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|text|string|true|none|none|

<h2 id="tocS_execapi_EmbedResponse">execapi_EmbedResponse</h2>
<!-- backwards compatibility -->
<a id="schemaexecapi_embedresponse"></a>
<a id="schema_execapi_EmbedResponse"></a>
<a id="tocSexecapi_embedresponse"></a>
<a id="tocsexecapi_embedresponse"></a>

```json
{
  "vector": "[0.1, 0.2, 0.3, ...]"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|vector|[number]|true|none|none|

<h2 id="tocS_execapi_taskExecutionRequest">execapi_taskExecutionRequest</h2>
<!-- backwards compatibility -->
<a id="schemaexecapi_taskexecutionrequest"></a>
<a id="schema_execapi_taskExecutionRequest"></a>
<a id="tocSexecapi_taskexecutionrequest"></a>
<a id="tocsexecapi_taskexecutionrequest"></a>

```json
{
  "chain": [
    {
      "debug": true,
      "description": "string",
      "id": "string",
      "tasks": [
        {
          "compose": {},
          "description": "Validates user input meets quality requirements",
          "execute_config": {},
          "handler": "condition_key",
          "hook": {},
          "id": "validate_input",
          "input_var": "input",
          "print": "Validation result: {{.validate_input}}",
          "prompt_template": "Is this input valid? {{.input}}",
          "retry_on_failure": 2,
          "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
          "timeout": "30s",
          "transition": {},
          "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
        }
      ],
      "token_limit": 0
    }
  ],
  "input": "What is the capital of France",
  "inputType": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|chain|array|true|none|none|
|input|object|true|none|none|
|inputType|string|true|none|none|

<h2 id="tocS_execapi_taskExecutionResponse">execapi_taskExecutionResponse</h2>
<!-- backwards compatibility -->
<a id="schemaexecapi_taskexecutionresponse"></a>
<a id="schema_execapi_taskExecutionResponse"></a>
<a id="tocSexecapi_taskexecutionresponse"></a>
<a id="tocsexecapi_taskexecutionresponse"></a>

```json
{
  "output": "Paris",
  "outputType": "string",
  "state": [
    {
      "duration": 452000000,
      "error": {},
      "input": "This is a test input that needs validation",
      "inputType": "string",
      "output": "valid",
      "outputType": "string",
      "taskHandler": "condition_key",
      "taskID": "validate_input",
      "transition": "valid_input"
    }
  ]
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|output|object|true|none|none|
|outputType|string|true|none|none|
|state|[[taskengine_CapturedStateUnit](#schemataskengine_capturedstateunit)]|true|none|none|

<h2 id="tocS_execservice_TaskRequest">execservice_TaskRequest</h2>
<!-- backwards compatibility -->
<a id="schemaexecservice_taskrequest"></a>
<a id="schema_execservice_TaskRequest"></a>
<a id="tocSexecservice_taskrequest"></a>
<a id="tocsexecservice_taskrequest"></a>

```json
{
  "model_name": "gpt-3.5-turbo",
  "model_provider": "openai",
  "prompt": "Hello, how are you?"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|model_name|string|true|none|none|
|model_provider|string|true|none|none|
|prompt|string|true|none|none|

<h2 id="tocS_runtimestate_ProviderConfig">runtimestate_ProviderConfig</h2>
<!-- backwards compatibility -->
<a id="schemaruntimestate_providerconfig"></a>
<a id="schema_runtimestate_ProviderConfig"></a>
<a id="tocSruntimestate_providerconfig"></a>
<a id="tocsruntimestate_providerconfig"></a>

```json
{
  "APIKey": "string",
  "Type": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|APIKey|string|false|none|none|
|Type|string|false|none|none|

<h2 id="tocS_runtimetypes_Backend">runtimetypes_Backend</h2>
<!-- backwards compatibility -->
<a id="schemaruntimetypes_backend"></a>
<a id="schema_runtimetypes_Backend"></a>
<a id="tocSruntimetypes_backend"></a>
<a id="tocsruntimetypes_backend"></a>

```json
{
  "baseUrl": "http://ollama-prod.internal:11434",
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "name": "ollama-production",
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|baseUrl|string|true|none|none|
|createdAt|string(date-time)|true|none|none|
|id|string|true|none|none|
|name|string|true|none|none|
|type|string|true|none|none|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_runtimetypes_Model">runtimetypes_Model</h2>
<!-- backwards compatibility -->
<a id="schemaruntimetypes_model"></a>
<a id="schema_runtimetypes_Model"></a>
<a id="tocSruntimetypes_model"></a>
<a id="tocsruntimetypes_model"></a>

```json
{
  "canChat": true,
  "canEmbed": false,
  "canPrompt": true,
  "canStream": true,
  "contextLength": 8192,
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "m7d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a",
  "model": "mistral:instruct",
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|canChat|boolean|true|none|none|
|canEmbed|boolean|true|none|none|
|canPrompt|boolean|true|none|none|
|canStream|boolean|true|none|none|
|contextLength|integer|true|none|none|
|createdAt|string(date-time)|true|none|none|
|id|string|true|none|none|
|model|string|true|none|none|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_runtimetypes_Pool">runtimetypes_Pool</h2>
<!-- backwards compatibility -->
<a id="schemaruntimetypes_pool"></a>
<a id="schema_runtimetypes_Pool"></a>
<a id="tocSruntimetypes_pool"></a>
<a id="tocsruntimetypes_pool"></a>

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "id": "p9a8b7c6-d5e4-f3a2-b1c0-d9e8f7a6b5c4",
  "name": "production-chat",
  "purposeType": "Internal Tasks",
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|createdAt|string(date-time)|true|none|none|
|id|string|true|none|none|
|name|string|true|none|none|
|purposeType|string|true|none|none|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_runtimetypes_RemoteHook">runtimetypes_RemoteHook</h2>
<!-- backwards compatibility -->
<a id="schemaruntimetypes_remotehook"></a>
<a id="schema_runtimetypes_RemoteHook"></a>
<a id="tocSruntimetypes_remotehook"></a>
<a id="tocsruntimetypes_remotehook"></a>

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "method": "POST",
  "name": "send-email",
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|createdAt|string(date-time)|true|none|none|
|endpointUrl|string|true|none|none|
|id|string|true|none|none|
|method|string|true|none|none|
|name|string|true|none|none|
|timeoutMs|integer|true|none|none|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_statetype_BackendRuntimeState">statetype_BackendRuntimeState</h2>
<!-- backwards compatibility -->
<a id="schemastatetype_backendruntimestate"></a>
<a id="schema_statetype_BackendRuntimeState"></a>
<a id="tocSstatetype_backendruntimestate"></a>
<a id="tocsstatetype_backendruntimestate"></a>

```json
{
  "backend": {},
  "error": "connection timeout: context deadline exceeded",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
  "name": "ollama-production",
  "pulledModels": []
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|backend|object|true|none|none|
|error|string|false|none|Error stores a description of the last encountered error when<br>interacting with or reconciling this backend's state, if any.|
|id|string|true|none|none|
|models|[string]|true|none|none|
|name|string|true|none|none|
|pulledModels|[[statetype_ListModelResponse](#schemastatetype_listmodelresponse)]|true|none|none|

<h2 id="tocS_statetype_ModelPullStatus">statetype_ModelPullStatus</h2>
<!-- backwards compatibility -->
<a id="schemastatetype_modelpullstatus"></a>
<a id="schema_statetype_ModelPullStatus"></a>
<a id="tocSstatetype_modelpullstatus"></a>
<a id="tocsstatetype_modelpullstatus"></a>

```json
{
  "canChat": true,
  "canEmbed": false,
  "canPrompt": true,
  "canStream": true,
  "contextLength": 4096,
  "details": {},
  "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
  "model": "mistral:instruct",
  "modifiedAt": "2023-11-15T14:30:45Z",
  "name": "Mistral 7B Instruct",
  "size": 4709611008
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|canChat|boolean|true|none|none|
|canEmbed|boolean|true|none|none|
|canPrompt|boolean|true|none|none|
|canStream|boolean|true|none|none|
|contextLength|integer|true|none|none|
|details|object|true|none|none|
|digest|string|true|none|none|
|model|string|true|none|none|
|modifiedAt|string(date-time)|true|none|none|
|name|string|true|none|none|
|size|integer|true|none|none|

<h2 id="tocS_taskengine_CapturedStateUnit">taskengine_CapturedStateUnit</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_capturedstateunit"></a>
<a id="schema_taskengine_CapturedStateUnit"></a>
<a id="tocStaskengine_capturedstateunit"></a>
<a id="tocstaskengine_capturedstateunit"></a>

```json
{
  "duration": 452000000,
  "error": {},
  "input": "This is a test input that needs validation",
  "inputType": "string",
  "output": "valid",
  "outputType": "string",
  "taskHandler": "condition_key",
  "taskID": "validate_input",
  "transition": "valid_input"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|duration|object|true|none|452ms in nanoseconds|
|error|object|true|none|none|
|input|string|true|none|none|
|inputType|object|true|none|none|
|output|string|true|none|none|
|outputType|object|true|none|none|
|taskHandler|string|true|none|none|
|taskID|string|true|none|none|
|transition|string|true|none|none|

<h2 id="tocS_taskengine_HookCall">taskengine_HookCall</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_hookcall"></a>
<a id="schema_taskengine_HookCall"></a>
<a id="tocStaskengine_hookcall"></a>
<a id="tocstaskengine_hookcall"></a>

```json
{
  "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
  "name": "slack_notification"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|args|object|true|none|Args are key-value pairs to parameterize the hook call.<br>Example: {"to": "user@example.com", "subject": "Notification"}|
|name|string|true|none|Name is the registered hook name to invoke (e.g., "send_email").|

<h2 id="tocS_taskengine_LLMExecutionConfig">taskengine_LLMExecutionConfig</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_llmexecutionconfig"></a>
<a id="schema_taskengine_LLMExecutionConfig"></a>
<a id="tocStaskengine_llmexecutionconfig"></a>
<a id="tocstaskengine_llmexecutionconfig"></a>

```json
{
  "model": "mistral:instruct",
  "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
  "provider": "ollama",
  "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
  "temperature": 0.7
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|model|string|true|none|none|
|models|[string]|false|none|none|
|provider|string|false|none|none|
|providers|[string]|false|none|none|
|temperature|number|false|none|none|

<h2 id="tocS_taskengine_TaskChainDefinition">taskengine_TaskChainDefinition</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_taskchaindefinition"></a>
<a id="schema_taskengine_TaskChainDefinition"></a>
<a id="tocStaskengine_taskchaindefinition"></a>
<a id="tocstaskengine_taskchaindefinition"></a>

```json
{
  "debug": true,
  "description": "string",
  "id": "string",
  "tasks": [
    {
      "compose": {},
      "description": "Validates user input meets quality requirements",
      "execute_config": {},
      "handler": "condition_key",
      "hook": {},
      "id": "validate_input",
      "input_var": "input",
      "print": "Validation result: {{.validate_input}}",
      "prompt_template": "Is this input valid? {{.input}}",
      "retry_on_failure": 2,
      "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
      "timeout": "30s",
      "transition": {},
      "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
    }
  ],
  "token_limit": 0
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|debug|boolean|true|none|Enables capturing user input and output.|
|description|string|true|none|Description provides a human-readable summary of the chain's purpose.|
|id|string|true|none|ID uniquely identifies the chain.|
|tasks|[[taskengine_TaskDefinition](#schemataskengine_taskdefinition)]|true|none|Tasks is the list of tasks to execute in sequence.|
|token_limit|integer|true|none|TokenLimit is the token limit for the context window (used during execution).|

<h2 id="tocS_taskengine_TaskDefinition">taskengine_TaskDefinition</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_taskdefinition"></a>
<a id="schema_taskengine_TaskDefinition"></a>
<a id="tocStaskengine_taskdefinition"></a>
<a id="tocstaskengine_taskdefinition"></a>

```json
{
  "compose": {},
  "description": "Validates user input meets quality requirements",
  "execute_config": {},
  "handler": "condition_key",
  "hook": {},
  "id": "validate_input",
  "input_var": "input",
  "print": "Validation result: {{.validate_input}}",
  "prompt_template": "Is this input valid? {{.input}}",
  "retry_on_failure": 2,
  "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
  "timeout": "30s",
  "transition": {},
  "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|compose|object|false|none|Compose merges the specified the output with the withVar side.<br>Optional. compose is applied before the input reaches the task execution,|
|description|string|true|none|Description is a human-readable summary of what the task does.|
|execute_config|object|false|none|ExecuteConfig defines the configuration for executing prompt or chat model tasks.|
|handler|object|true|none|Handler determines how the LLM output (or hook) will be interpreted.|
|hook|object|false|none|Hook defines an external action to run.<br>Required for Hook tasks, must be nil/omitted for all other types.<br>Example: {type: "send_email", args: {"to": "user@example.com"}}|
|id|string|true|none|ID uniquely identifies the task within the chain.|
|input_var|string|false|none|InputVar is the name of the variable to use as input for the task.<br>Example: "input" for the original input.<br>Each task stores its output in a variable named with it's task id.|
|print|string|false|none|Print optionally formats the output for display/logging.<br>Supports template variables from previous task outputs.<br>Optional for all task types except Hook where it's rarely used.<br>Example: "The score is: {{.previous_output}}"|
|prompt_template|string|true|none|PromptTemplate is the text prompt sent to the LLM.<br>It's Required and only applicable for the raw_string type.<br>Supports template variables from previous task outputs.<br>Example: "Rate the quality from 1-10: {{.input}}"|
|retry_on_failure|integer|false|none|RetryOnFailure sets how many times to retry this task on failure.<br>Applies to all task types including Hooks.<br>Default: 0 (no retries)|
|system_instruction|string|false|none|SystemInstruction provides additional instructions to the LLM, if applicable system level will be used.|
|timeout|string|false|none|Timeout optionally sets a timeout for task execution.<br>Format: "10s", "2m", "1h" etc.<br>Optional for all task types.|
|transition|object|true|none|Transition defines what to do after this task completes.|
|valid_conditions|object|false|none|ValidConditions defines allowed values for ConditionKey tasks.<br>Required for ConditionKey tasks, ignored for all other types.<br>Example: {"yes": true, "no": true} for a yes/no condition.|

<h2 id="tocS_taskengine_TaskTransition">taskengine_TaskTransition</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_tasktransition"></a>
<a id="schema_taskengine_TaskTransition"></a>
<a id="tocStaskengine_tasktransition"></a>
<a id="tocstaskengine_tasktransition"></a>

```json
{
  "branches": [
    {
      "alert_on_match": "Positive response detected: {{.output}}",
      "goto": "positive_response",
      "operator": "equals",
      "when": "yes"
    }
  ],
  "on_failure": "error_handler",
  "on_failure_alert": "Task failed: {{.error}}"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|branches|[[taskengine_TransitionBranch](#schemataskengine_transitionbranch)]|true|none|Branches defines conditional branches for successful task completion.|
|on_failure|string|true|none|OnFailure is the task ID to jump to in case of failure.|
|on_failure_alert|string|true|none|OnFailureAlert specifies the alert message to send if the task fails.|

<h2 id="tocS_taskengine_TransitionBranch">taskengine_TransitionBranch</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_transitionbranch"></a>
<a id="schema_taskengine_TransitionBranch"></a>
<a id="tocStaskengine_transitionbranch"></a>
<a id="tocstaskengine_transitionbranch"></a>

```json
{
  "alert_on_match": "Positive response detected: {{.output}}",
  "goto": "positive_response",
  "operator": "equals",
  "when": "yes"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|alert_on_match|string|true|none|AlertOnMatch specifies the alert message to send if this branch is taken.|
|goto|string|true|none|Goto specifies the target task ID if this branch is taken.<br>Leave empty or use taskengine.TermEnd to end the chain.|
|operator|object|false|none|Operator defines how to compare the task's output to When.|
|when|string|true|none|When specifies the condition that must be met to follow this branch.<br>Format depends on the task type:<br>- For condition_key: exact string match<br>- For parse_number: numeric comparison (using Operator)|


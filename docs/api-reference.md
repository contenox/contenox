---
title: contenox/runtime – LLM Backend Management API v0.0.51-146-g68c140e-dirty
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

<h1 id="contenox-runtime-llm-backend-management-api">contenox/runtime – LLM Backend Management API v0.0.51-146-g68c140e-dirty</h1>

> Scroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

# Authentication

* API Key (X-API-Key)
    - Parameter Name: **X-API-Key**, in: header. 

<h1 id="contenox-runtime-llm-backend-management-api-default">Default</h1>

## Lists all affinity groups that a specific backend belongs to.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/backend-affinity/{backendID}/groups', headers = headers)

print(r.json())

```

`GET /backend-affinity/{backendID}/groups`

Lists all affinity groups that a specific backend belongs to.
Useful for understanding which model sets a backend has access to.

<h3 id="lists-all-affinity-groups-that-a-specific-backend-belongs-to.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|backendID|path|string|true|The unique identifier of the backend.|

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

<h3 id="lists-all-affinity-groups-that-a-specific-backend-belongs-to.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_AffinityGroup](#schemaarray_runtimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all backends associated with a specific affinity group.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/backend-affinity/{groupID}/backends', headers = headers)

print(r.json())

```

`GET /backend-affinity/{groupID}/backends`

Lists all backends associated with a specific affinity group.
Returns basic backend information without runtime state.

<h3 id="lists-all-backends-associated-with-a-specific-affinity-group.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|groupID|path|string|true|The unique identifier of the affinity group.|

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

<h3 id="lists-all-backends-associated-with-a-specific-affinity-group.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Backend](#schemaarray_runtimetypes_backend)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Removes a backend from an affinity group.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/backend-affinity/{groupID}/backends/{backendID}', headers = headers)

print(r.json())

```

`DELETE /backend-affinity/{groupID}/backends/{backendID}`

Removes a backend from an affinity group.
After removal, the backend will no longer be eligible to process requests for models in this affinity group.
Requests requiring models from this affinity group will no longer be routed to this backend.

<h3 id="removes-a-backend-from-an-affinity-group.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|groupID|path|string|true|The unique identifier of the affinity group.|
|backendID|path|string|true|The unique identifier of the backend to be assigned.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removes-a-backend-from-an-affinity-group.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Associates a backend with an affinity group.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/backend-affinity/{groupID}/backends/{backendID}', headers = headers)

print(r.json())

```

`POST /backend-affinity/{groupID}/backends/{backendID}`

Associates a backend with an affinity group.
After assignment, the backend can process requests for all models in the affinity group.
This enables request routing between the backend and models that share this affinity group.

<h3 id="associates-a-backend-with-an-affinity-group.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|groupID|path|string|true|The unique identifier of the affinity group.|
|backendID|path|string|true|The unique identifier of the backend to be assigned.|

> Example responses

> 201 Response

```json
"string"
```

<h3 id="associates-a-backend-with-an-affinity-group.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all configured backend connections with runtime status.

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
NOTE: Only backends assigned to at least one group will be used for request processing.
Backends not assigned to any group exist in the configuration but are completely ignored by the routing system.

<h3 id="lists-all-configured-backend-connections-with-runtime-status.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|

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
    "pulledModels": {
      "canChat": true,
      "canEmbed": false,
      "canPrompt": true,
      "canStream": true,
      "contextLength": 4096,
      "details": {
        "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
        "family": "Mistral",
        "format": "gguf",
        "parameterSize": "7B",
        "parentModel": "mistral:7b",
        "quantizationLevel": "Q4_K_M"
      },
      "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
      "model": "mistral:instruct",
      "modifiedAt": "2023-11-15T14:30:45Z",
      "name": "Mistral 7B Instruct",
      "size": 4709611008
    },
    "type": "ollama",
    "updatedAt": "2023-01-01T00:00:00Z"
  }
]
```

<h3 id="lists-all-configured-backend-connections-with-runtime-status.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_backendapi_backendSummary](#schemaarray_backendapi_backendsummary)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Creates a new backend connection to an LLM provider.

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

<h3 id="creates-a-new-backend-connection-to-an-llm-provider.-parameters">Parameters</h3>

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

<h3 id="creates-a-new-backend-connection-to-an-llm-provider.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_Backend](#schemaruntimetypes_backend)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Removes a backend connection.

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

<h3 id="removes-a-backend-connection.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier for the backend.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removes-a-backend-connection.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves complete information for a specific backend

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

<h3 id="retrieves-complete-information-for-a-specific-backend-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier for the backend.|

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
  "pulledModels": {
    "canChat": true,
    "canEmbed": false,
    "canPrompt": true,
    "canStream": true,
    "contextLength": 4096,
    "details": {
      "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
      "family": "Mistral",
      "format": "gguf",
      "parameterSize": "7B",
      "parentModel": "mistral:7b",
      "quantizationLevel": "Q4_K_M"
    },
    "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
    "model": "mistral:instruct",
    "modifiedAt": "2023-11-15T14:30:45Z",
    "name": "Mistral 7B Instruct",
    "size": 4709611008
  },
  "type": "ollama",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="retrieves-complete-information-for-a-specific-backend-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[backendapi_backendDetails](#schemabackendapi_backenddetails)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Updates an existing backend configuration.

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

<h3 id="updates-an-existing-backend-configuration.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Backend](#schemaruntimetypes_backend)|true|none|
|id|path|string|true|The unique identifier for the backend.|

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

<h3 id="updates-an-existing-backend-configuration.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_Backend](#schemaruntimetypes_backend)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Returns the default model configured during system initialization.

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

<h3 id="returns-the-default-model-configured-during-system-initialization.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[execapi_DefaultModelResponse](#schemaexecapi_defaultmodelresponse)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Generates vector embeddings for text.

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
Requests are routed ONLY to backends that have the default model available in any shared group.
If groups are enabled, models and backends not assigned to any group will be completely ignored by the routing system.

> Body parameter

```json
{
  "text": "Hello, world!"
}
```

<h3 id="generates-vector-embeddings-for-text.-parameters">Parameters</h3>

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

<h3 id="generates-vector-embeddings-for-text.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[execapi_EmbedResponse](#schemaexecapi_embedresponse)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all event triggers with pagination

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/event-triggers', headers = headers)

print(r.json())

```

`GET /event-triggers`

Lists all event triggers with pagination
Returns event triggers in creation order, with the oldest triggers first.

<h3 id="lists-all-event-triggers-with-pagination-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "description": "Send a welcome email to a new user",
    "function": "new_user_created_event_handler",
    "listenFor": {
      "type": "contenox.user_created"
    },
    "name": "new_user_created",
    "type": "function",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="lists-all-event-triggers-with-pagination-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_functionstore_EventTrigger](#schemaarray_functionstore_eventtrigger)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Creates a new event trigger

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/event-triggers', headers = headers)

print(r.json())

```

`POST /event-triggers`

Creates a new event trigger
Event triggers listen for specific events and execute associated functions.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "Send a welcome email to a new user",
  "function": "new_user_created_event_handler",
  "listenFor": {
    "type": "contenox.user_created"
  },
  "name": "new_user_created",
  "type": "function",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="creates-a-new-event-trigger-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[functionstore_EventTrigger](#schemafunctionstore_eventtrigger)|true|none|

> Example responses

> 201 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "Send a welcome email to a new user",
  "function": "new_user_created_event_handler",
  "listenFor": {
    "type": "contenox.user_created"
  },
  "name": "new_user_created",
  "type": "function",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="creates-a-new-event-trigger-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[functionstore_EventTrigger](#schemafunctionstore_eventtrigger)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists event triggers filtered by event type

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/event-triggers/event-type/{eventType}', headers = headers)

print(r.json())

```

`GET /event-triggers/event-type/{eventType}`

Lists event triggers filtered by event type
Returns all event triggers that listen for the specified event type.

<h3 id="lists-event-triggers-filtered-by-event-type-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|eventType|path|string|true|The event type to filter by.|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "description": "Send a welcome email to a new user",
    "function": "new_user_created_event_handler",
    "listenFor": {
      "type": "contenox.user_created"
    },
    "name": "new_user_created",
    "type": "function",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="lists-event-triggers-filtered-by-event-type-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_functionstore_EventTrigger](#schemaarray_functionstore_eventtrigger)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists event triggers filtered by function name

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/event-triggers/function/{functionName}', headers = headers)

print(r.json())

```

`GET /event-triggers/function/{functionName}`

Lists event triggers filtered by function name
Returns all event triggers that execute the specified function.

<h3 id="lists-event-triggers-filtered-by-function-name-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|functionName|path|string|true|The function name to filter by.|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "description": "Send a welcome email to a new user",
    "function": "new_user_created_event_handler",
    "listenFor": {
      "type": "contenox.user_created"
    },
    "name": "new_user_created",
    "type": "function",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="lists-event-triggers-filtered-by-function-name-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_functionstore_EventTrigger](#schemaarray_functionstore_eventtrigger)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Deletes an event trigger from the system

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/event-triggers/{name}', headers = headers)

print(r.json())

```

`DELETE /event-triggers/{name}`

Deletes an event trigger from the system
Returns a simple confirmation message on success.

<h3 id="deletes-an-event-trigger-from-the-system-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|The unique name of the event trigger.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletes-an-event-trigger-from-the-system-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves details for a specific event trigger

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/event-triggers/{name}', headers = headers)

print(r.json())

```

`GET /event-triggers/{name}`

Retrieves details for a specific event trigger

<h3 id="retrieves-details-for-a-specific-event-trigger-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|The unique name of the event trigger.|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "Send a welcome email to a new user",
  "function": "new_user_created_event_handler",
  "listenFor": {
    "type": "contenox.user_created"
  },
  "name": "new_user_created",
  "type": "function",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="retrieves-details-for-a-specific-event-trigger-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[functionstore_EventTrigger](#schemafunctionstore_eventtrigger)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Updates an existing event trigger configuration

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/event-triggers/{name}', headers = headers)

print(r.json())

```

`PUT /event-triggers/{name}`

Updates an existing event trigger configuration
The name from the URL path overrides any name in the request body.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "Send a welcome email to a new user",
  "function": "new_user_created_event_handler",
  "listenFor": {
    "type": "contenox.user_created"
  },
  "name": "new_user_created",
  "type": "function",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updates-an-existing-event-trigger-configuration-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[functionstore_EventTrigger](#schemafunctionstore_eventtrigger)|true|none|
|name|path|string|true|The unique name of the event trigger.|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "Send a welcome email to a new user",
  "function": "new_user_created_event_handler",
  "listenFor": {
    "type": "contenox.user_created"
  },
  "name": "new_user_created",
  "type": "function",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updates-an-existing-event-trigger-configuration-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[functionstore_EventTrigger](#schemafunctionstore_eventtrigger)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Appends a new event to the event store.

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/events', headers = headers)

print(r.json())

```

`POST /events`

Appends a new event to the event store.
The event ID and CreatedAt will be auto-generated if not provided.
Events must be within ±10 minutes of current server time.

> Body parameter

```json
{
  "aggregate_id": "aggregate-uuid",
  "aggregate_type": "github.webhook",
  "created_at": "2023-01-01T00:00:00Z",
  "data": {},
  "event_source": "github.com",
  "event_type": "github.pull_request",
  "id": "event-uuid",
  "metadata": {},
  "nid": 1,
  "version": 1
}
```

<h3 id="appends-a-new-event-to-the-event-store.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[eventstore_Event](#schemaeventstore_event)|true|none|

> Example responses

> 201 Response

```json
{
  "aggregate_id": "aggregate-uuid",
  "aggregate_type": "github.webhook",
  "created_at": "2023-01-01T00:00:00Z",
  "data": {},
  "event_source": "github.com",
  "event_type": "github.pull_request",
  "id": "event-uuid",
  "metadata": {},
  "nid": 1,
  "version": 1
}
```

<h3 id="appends-a-new-event-to-the-event-store.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[eventstore_Event](#schemaeventstore_event)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves events for a specific aggregate within a time range.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/events/aggregate', headers = headers)

print(r.json())

```

`GET /events/aggregate`

Retrieves events for a specific aggregate within a time range.
Useful for rebuilding aggregate state or auditing changes.

<h3 id="retrieves-events-for-a-specific-aggregate-within-a-time-range.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|aggregate_type|query|string|false|The aggregate type (e.g., 'user', 'order').|
|aggregate_id|query|string|false|The unique ID of the aggregate.|
|limit|query|string|false|Maximum number of events to return.|
|event_type|query|string|false|The type of event to filter by.|

> Example responses

> 200 Response

```json
[
  {
    "aggregate_id": "aggregate-uuid",
    "aggregate_type": "github.webhook",
    "created_at": "2023-01-01T00:00:00Z",
    "data": {},
    "event_source": "github.com",
    "event_type": "github.pull_request",
    "id": "event-uuid",
    "metadata": {},
    "nid": 1,
    "version": 1
  }
]
```

<h3 id="retrieves-events-for-a-specific-aggregate-within-a-time-range.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_eventstore_Event](#schemaarray_eventstore_event)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves events from a specific source within a time range.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/events/source', headers = headers)

print(r.json())

```

`GET /events/source`

Retrieves events from a specific source within a time range.
Useful for auditing or monitoring events from specific subsystems.

<h3 id="retrieves-events-from-a-specific-source-within-a-time-range.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|event_type|query|string|false|The type of event to filter by.|
|event_source|query|string|false|The source system that generated the event.|
|limit|query|string|false|Maximum number of events to return.|

> Example responses

> 200 Response

```json
[
  {
    "aggregate_id": "aggregate-uuid",
    "aggregate_type": "github.webhook",
    "created_at": "2023-01-01T00:00:00Z",
    "data": {},
    "event_source": "github.com",
    "event_type": "github.pull_request",
    "id": "event-uuid",
    "metadata": {},
    "nid": 1,
    "version": 1
  }
]
```

<h3 id="retrieves-events-from-a-specific-source-within-a-time-range.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_eventstore_Event](#schemaarray_eventstore_event)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Streams events of a specific type in real-time using Server-Sent Events (SSE)

> Code samples

```python
import requests
headers = {
  'Accept': 'text/event-stream',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/events/stream/{eventType}', headers = headers)

print(r.json())

```

`GET /events/stream/{eventType}`

Streams events of a specific type in real-time using Server-Sent Events (SSE)
This endpoint provides real-time event streaming for the specified event type.
Clients will receive new events as they are appended to the event store.
--- SSE Streaming ---
The endpoint streams events using Server-Sent Events (SSE) format.
Each event is sent as a JSON object in the data field.
Example event stream:
data: {"id":"evt_123","event_type":"user_created","aggregate_type":"user","aggregate_id":"usr_456","version":1,"data":{"name":"John Doe"},"created_at":"2023-01-01T00:00:00Z"}
data: {"id":"evt_124","event_type":"user_updated","aggregate_type":"user","aggregate_id":"usr_456","version":2,"data":{"name":"Jane Doe"},"created_at":"2023-01-01T00:01:00Z"}

<h3 id="streams-events-of-a-specific-type-in-real-time-using-server-sent-events-(sse)-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|eventType|path|string|true|The type of events to stream.|

> Example responses

> 200 Response

> default Response

```json
{
  "error": {
    "code": "string",
    "message": "string",
    "param": "string",
    "type": "string"
  }
}
```

<h3 id="streams-events-of-a-specific-type-in-real-time-using-server-sent-events-(sse)-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Deletes all events of a specific type within a time range.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/events/type', headers = headers)

print(r.json())

```

`DELETE /events/type`

Deletes all events of a specific type within a time range.
USE WITH CAUTION — this is a destructive operation.
Typically used for GDPR compliance or cleaning up test data.

<h3 id="deletes-all-events-of-a-specific-type-within-a-time-range.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|event_type|query|string|false|The type of event to delete.|
|from|query|string|false|Start time in RFC3339 format.|
|to|query|string|false|End time in RFC3339 format.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletes-all-events-of-a-specific-type-within-a-time-range.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves events of a specific type within a time range.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/events/type', headers = headers)

print(r.json())

```

`GET /events/type`

Retrieves events of a specific type within a time range.
Useful for cross-aggregate analysis or system-wide event monitoring.

<h3 id="retrieves-events-of-a-specific-type-within-a-time-range.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|Maximum number of events to return.|
|event_type|query|string|false|The type of event to filter by.|

> Example responses

> 200 Response

```json
[
  {
    "aggregate_id": "aggregate-uuid",
    "aggregate_type": "github.webhook",
    "created_at": "2023-01-01T00:00:00Z",
    "data": {},
    "event_source": "github.com",
    "event_type": "github.pull_request",
    "id": "event-uuid",
    "metadata": {},
    "nid": 1,
    "version": 1
  }
]
```

<h3 id="retrieves-events-of-a-specific-type-within-a-time-range.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_eventstore_Event](#schemaarray_eventstore_event)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists distinct event types that occurred within a time range.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/events/types', headers = headers)

print(r.json())

```

`GET /events/types`

Lists distinct event types that occurred within a time range.
Useful for discovery or building event type filters in UIs.

<h3 id="lists-distinct-event-types-that-occurred-within-a-time-range.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|Maximum number of event types to return.|

> Example responses

> 200 Response

```json
[
  "string"
]
```

<h3 id="lists-distinct-event-types-that-occurred-within-a-time-range.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_string](#schemaarray_string)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Runs the prompt through the default LLM.

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
Requests are routed ONLY to backends that have the default model available in any shared group.
If groups are enabled, models and backends not assigned to any group will be completely ignored by the routing system.

> Body parameter

```json
{
  "model_name": "gpt-3.5-turbo",
  "model_provider": "openai",
  "prompt": "Hello, how are you?"
}
```

<h3 id="runs-the-prompt-through-the-default-llm.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[execservice_TaskRequest](#schemaexecservice_taskrequest)|true|none|

> Example responses

> 200 Response

```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "response": "I'm doing well, thank you!"
}
```

<h3 id="runs-the-prompt-through-the-default-llm.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[execservice_SimpleExecutionResponse](#schemaexecservice_simpleexecutionresponse)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all registered functions with pagination

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/functions', headers = headers)

print(r.json())

```

`GET /functions`

Lists all registered functions with pagination
Returns functions in creation order, with the oldest functions first.

<h3 id="lists-all-registered-functions-with-pagination-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "description": "string",
    "name": "send_welcome_email_event_handler",
    "script": "string",
    "scriptType": "goja",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="lists-all-registered-functions-with-pagination-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_functionstore_Function](#schemaarray_functionstore_function)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Creates a new serverless function

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/functions', headers = headers)

print(r.json())

```

`POST /functions`

Creates a new serverless function
Functions contain executable JavaScript code that runs in a secure sandbox.
After execution, functions can trigger chains for further processing.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "string",
  "name": "send_welcome_email_event_handler",
  "script": "string",
  "scriptType": "goja",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="creates-a-new-serverless-function-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[functionstore_Function](#schemafunctionstore_function)|true|none|

> Example responses

> 201 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "string",
  "name": "send_welcome_email_event_handler",
  "script": "string",
  "scriptType": "goja",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="creates-a-new-serverless-function-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[functionstore_Function](#schemafunctionstore_function)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Deletes a function from the system

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/functions/{name}', headers = headers)

print(r.json())

```

`DELETE /functions/{name}`

Deletes a function from the system
Returns a simple confirmation message on success.

<h3 id="deletes-a-function-from-the-system-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|The unique name of the function.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletes-a-function-from-the-system-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves details for a specific function

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/functions/{name}', headers = headers)

print(r.json())

```

`GET /functions/{name}`

Retrieves details for a specific function

<h3 id="retrieves-details-for-a-specific-function-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|The unique name of the function.|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "string",
  "name": "send_welcome_email_event_handler",
  "script": "string",
  "scriptType": "goja",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="retrieves-details-for-a-specific-function-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[functionstore_Function](#schemafunctionstore_function)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Updates an existing function configuration

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/functions/{name}', headers = headers)

print(r.json())

```

`PUT /functions/{name}`

Updates an existing function configuration
The name from the URL path overrides any name in the request body.

> Body parameter

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "string",
  "name": "send_welcome_email_event_handler",
  "script": "string",
  "scriptType": "goja",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updates-an-existing-function-configuration-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[functionstore_Function](#schemafunctionstore_function)|true|none|
|name|path|string|true|The unique name of the function.|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "string",
  "name": "send_welcome_email_event_handler",
  "script": "string",
  "scriptType": "goja",
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updates-an-existing-function-configuration-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[functionstore_Function](#schemafunctionstore_function)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves an affinity group by its human-readable name.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/group-by-name/{name}', headers = headers)

print(r.json())

```

`GET /group-by-name/{name}`

Retrieves an affinity group by its human-readable name.
Useful for configuration where ID might not be known but name is consistent.

<h3 id="retrieves-an-affinity-group-by-its-human-readable-name.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|The unique, human-readable name of the affinity group.|

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

<h3 id="retrieves-an-affinity-group-by-its-human-readable-name.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_AffinityGroup](#schemaruntimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists groups filtered by purpose type with pagination support.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/group-by-purpose/{purpose}', headers = headers)

print(r.json())

```

`GET /group-by-purpose/{purpose}`

Lists groups filtered by purpose type with pagination support.
Purpose types categorize groups (e.g., "Internal Embeddings", "Internal Tasks").
Accepts 'cursor' (RFC3339Nano timestamp) and 'limit' parameters for pagination.

<h3 id="lists-groups-filtered-by-purpose-type-with-pagination-support.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|
|purpose|path|string|true|The purpose category to filter groups by (e.g., 'embeddings').|

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

<h3 id="lists-groups-filtered-by-purpose-type-with-pagination-support.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_AffinityGroup](#schemaarray_runtimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all affinity groups in the system.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/groups', headers = headers)

print(r.json())

```

`GET /groups`

Lists all affinity groups in the system.
Returns basic group information without associated backends or models.

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

<h3 id="lists-all-affinity-groups-in-the-system.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_AffinityGroup](#schemaarray_runtimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Creates a new affinity group for organizing backends and models.

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/groups', headers = headers)

print(r.json())

```

`POST /groups`

Creates a new affinity group for organizing backends and models.
group names must be unique within the system.
groups allow grouping of backends and models for specific operational purposes (e.g., embeddings, tasks).
When affinity groups are enabled in the system, request routing ONLY considers resources that share a affinity group.
- Models not assigned to any group will NOT be available for execution
- Backends not assigned to any group will NOT receive models or process requests
- Resources must be explicitly associated with the same group to work together
This is a fundamental operational requirement - resources outside groups are effectively invisible to the routing system.

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

<h3 id="creates-a-new-affinity-group-for-organizing-backends-and-models.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_AffinityGroup](#schemaruntimetypes_affinitygroup)|true|none|

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

<h3 id="creates-a-new-affinity-group-for-organizing-backends-and-models.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_AffinityGroup](#schemaruntimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Removes an affinity group from the system.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/groups/{id}', headers = headers)

print(r.json())

```

`DELETE /groups/{id}`

Removes an affinity group from the system.
This does not delete the group's backends or models, only the group relationship.
Returns a simple "deleted" confirmation message on success.

<h3 id="removes-an-affinity-group-from-the-system.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier of the affinity group.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removes-an-affinity-group-from-the-system.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves an specific affinity group by its unique ID.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/groups/{id}', headers = headers)

print(r.json())

```

`GET /groups/{id}`

Retrieves an specific affinity group by its unique ID.

<h3 id="retrieves-an-specific-affinity-group-by-its-unique-id.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier of the affinity group.|

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

<h3 id="retrieves-an-specific-affinity-group-by-its-unique-id.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_AffinityGroup](#schemaruntimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Updates an existing affinity group configuration.

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/groups/{id}', headers = headers)

print(r.json())

```

`PUT /groups/{id}`

Updates an existing affinity group configuration.
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

<h3 id="updates-an-existing-affinity-group-configuration.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_AffinityGroup](#schemaruntimetypes_affinitygroup)|true|none|
|id|path|string|true|The unique identifier of the affinity group.|

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

<h3 id="updates-an-existing-affinity-group-configuration.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_AffinityGroup](#schemaruntimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists remote hooks, optionally filtering by a unique name.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/hooks/remote', headers = headers)

print(r.json())

```

`GET /hooks/remote`

Lists remote hooks, optionally filtering by a unique name.

<h3 id="lists-remote-hooks,-optionally-filtering-by-a-unique-name.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "endpointUrl": "http://hooks-endpoint:port",
    "headers": "Authorization:Bearer token,Content-Type:application/json",
    "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
    "name": "mailing-tools",
    "properties": {
      "in": "body",
      "name": "access_token",
      "value": null
    },
    "timeoutMs": 5000,
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]
```

<h3 id="lists-remote-hooks,-optionally-filtering-by-a-unique-name.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_RemoteHook](#schemaarray_runtimetypes_remotehook)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Creates a new remote hook configuration.

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
  "headers": "Authorization:Bearer token,Content-Type:application/json",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "name": "mailing-tools",
  "properties": {
    "in": "body",
    "name": "access_token",
    "value": null
  },
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="creates-a-new-remote-hook-configuration.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|true|none|

> Example responses

> 201 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "headers": "Authorization:Bearer token,Content-Type:application/json",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "name": "mailing-tools",
  "properties": {
    "in": "body",
    "name": "access_token",
    "value": null
  },
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="creates-a-new-remote-hook-configuration.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## getByName

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/hooks/remote/by-name/{name}', headers = headers)

print(r.json())

```

`GET /hooks/remote/by-name/{name}`

<h3 id="getbyname-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|name|path|string|true|The unique name for the remote hook.|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "headers": "Authorization:Bearer token,Content-Type:application/json",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "name": "mailing-tools",
  "properties": {
    "in": "body",
    "name": "access_token",
    "value": null
  },
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="getbyname-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Deletes a remote hook configuration by ID.

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

<h3 id="deletes-a-remote-hook-configuration-by-id.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier for the remote hook.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletes-a-remote-hook-configuration-by-id.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves a specific remote hook configuration by ID.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/hooks/remote/{id}', headers = headers)

print(r.json())

```

`GET /hooks/remote/{id}`

Retrieves a specific remote hook configuration by ID.

<h3 id="retrieves-a-specific-remote-hook-configuration-by-id.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier for the remote hook.|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "headers": "Authorization:Bearer token,Content-Type:application/json",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "name": "mailing-tools",
  "properties": {
    "in": "body",
    "name": "access_token",
    "value": null
  },
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="retrieves-a-specific-remote-hook-configuration-by-id.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Updates an existing remote hook configuration.

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
  "headers": "Authorization:Bearer token,Content-Type:application/json",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "name": "mailing-tools",
  "properties": {
    "in": "body",
    "name": "access_token",
    "value": null
  },
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updates-an-existing-remote-hook-configuration.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|true|none|
|id|path|string|true|The unique identifier for the remote hook.|

> Example responses

> 200 Response

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "endpointUrl": "http://hooks-endpoint:port",
  "headers": "Authorization:Bearer token,Content-Type:application/json",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "name": "mailing-tools",
  "properties": {
    "in": "body",
    "name": "access_token",
    "value": null
  },
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}
```

<h3 id="updates-an-existing-remote-hook-configuration.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_RemoteHook](#schemaruntimetypes_remotehook)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves the JSON openAPI schemas for all supported hook types.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/hooks/schemas', headers = headers)

print(r.json())

```

`GET /hooks/schemas`

Retrieves the JSON openAPI schemas for all supported hook types.

> Example responses

> 200 Response

```json
{}
```

<h3 id="retrieves-the-json-openapi-schemas-for-all-supported-hook-types.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|Inline|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<h3 id="retrieves-the-json-openapi-schemas-for-all-supported-hook-types.-responseschema">Response Schema</h3>

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all models associated with a specific affinity group.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/model-affinity/{groupID}/models', headers = headers)

print(r.json())

```

`GET /model-affinity/{groupID}/models`

Lists all models associated with a specific affinity group.
Returns basic model information without backend-specific details.

<h3 id="lists-all-models-associated-with-a-specific-affinity-group.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|groupID|path|string|true|The unique identifier of the affinity group.|

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

<h3 id="lists-all-models-associated-with-a-specific-affinity-group.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Model](#schemaarray_runtimetypes_model)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Removes a model from an affinity group.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/model-affinity/{groupID}/models/{modelID}', headers = headers)

print(r.json())

```

`DELETE /model-affinity/{groupID}/models/{modelID}`

Removes a model from an affinity group.
After removal, requests for this model will no longer be routed to backends in this affinity group.
This model can still be used with backends in other groups where it remains assigned.

<h3 id="removes-a-model-from-an-affinity-group.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|groupID|path|string|true|The unique identifier of the affinity group.|
|modelID|path|string|true|The unique identifier of the model to be assigned.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removes-a-model-from-an-affinity-group.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Associates a model with an affinity group.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/model-affinity/{groupID}/models/{modelID}', headers = headers)

print(r.json())

```

`POST /model-affinity/{groupID}/models/{modelID}`

Associates a model with an affinity group.
After assignment, requests for this model can be routed to any backend in the affinity group.
This enables request routing between the model and backends that share this affinity group.

<h3 id="associates-a-model-with-an-affinity-group.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|groupID|path|string|true|The unique identifier of the affinity group.|
|modelID|path|string|true|The unique identifier of the model to be assigned.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="associates-a-model-with-an-affinity-group.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all affinity groups that a specific model belongs to.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/model-affinity/{modelID}/groups', headers = headers)

print(r.json())

```

`GET /model-affinity/{modelID}/groups`

Lists all affinity groups that a specific model belongs to.
Useful for understanding where a model is deployed across the system.

<h3 id="lists-all-affinity-groups-that-a-specific-model-belongs-to.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|modelID|path|string|true|The unique identifier of the model.|

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

<h3 id="lists-all-affinity-groups-that-a-specific-model-belongs-to.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_AffinityGroup](#schemaarray_runtimetypes_affinitygroup)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all registered models in internal format.

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

Lists all registered models in internal format.
This endpoint returns full model details including timestamps and capabilities.
Intended for administrative and debugging purposes.

<h3 id="lists-all-registered-models-in-internal-format.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|

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

<h3 id="lists-all-registered-models-in-internal-format.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimetypes_Model](#schemaarray_runtimetypes_model)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Declares a new model to the system.

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
IMPORTANT: Models not assigned to any group will NOT be available for request processing.
If groups are enabled, to make a model available to backends, it must be explicitly added to at least one group.

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

<h3 id="declares-a-new-model-to-the-system.-parameters">Parameters</h3>

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

<h3 id="declares-a-new-model-to-the-system.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[runtimetypes_Model](#schemaruntimetypes_model)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Deletes a model from the system registry.

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

<h3 id="deletes-a-model-from-the-system-registry.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|purge|query|string|false|If true, also removes the model from the download queue and cancels any in-progress downloads.|
|id|path|string|true|The unique identifier for the model.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletes-a-model-from-the-system-registry.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Updates an existing model registration.

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

<h3 id="updates-an-existing-model-registration.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[runtimetypes_Model](#schemaruntimetypes_model)|true|none|
|id|path|string|true|The unique identifier for the model.|

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

<h3 id="updates-an-existing-model-registration.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimetypes_Model](#schemaruntimetypes_model)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all registered models in OpenAI-compatible format.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/openai/{chainID}/v1/models', headers = headers)

print(r.json())

```

`GET /openai/{chainID}/v1/models`

Lists all registered models in OpenAI-compatible format.
Returns models as they would appear in OpenAI's /v1/models endpoint.
NOTE: Only models assigned to at least one group will be available for request processing.
Models not assigned to any group exist in the configuration but are completely ignored by the routing system.
the chainID parameter is currently unused.

<h3 id="lists-all-registered-models-in-openai-compatible-format.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|
|chainID|path|string|true|The ID of the chain that links to the openAI completion API. Currently unused.|

> Example responses

> 200 Response

```json
{
  "data": [
    {}
  ],
  "object": "list"
}
```

<h3 id="lists-all-registered-models-in-openai-compatible-format.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[backendapi_OpenAICompatibleModelList](#schemabackendapi_openaicompatiblemodellist)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Processes chat requests using the configured task chain.

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/openai/{chainID}/v1/chat/completions', headers = headers)

print(r.json())

```

`POST /openai/{chainID}/v1/chat/completions`

Processes chat requests using the configured task chain.
This endpoint provides OpenAI-compatible chat completions by executing
the configured task chain with the provided request data.
The task chain must be configured first using the /chat/taskchain endpoint.
--- SSE Streaming ---
When 'stream: true' is set in the request body, the endpoint streams the response
using Server-Sent Events (SSE) in the OpenAI-compatible format.
Clients should concatenate the 'content' from the 'delta' object in each 'data' event
to reconstruct the full message. The stream is terminated by a 'data: [DONE]' message.
Example event stream:
data: {"id":"chat_123","object":"chat.completion.chunk","created":1690000000,"model":"mistral:instruct","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}
data: {"id":"chat_123","object":"chat.completion.chunk","created":1690000000,"model":"mistral:instruct","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}
data: [DONE]

> Body parameter

```json
{
  "frequency_penalty": 0,
  "max_tokens": 512,
  "messages": {
    "content": "Hello, how are you?",
    "role": "user"
  },
  "model": "mistral:instruct",
  "n": 1,
  "presence_penalty": 0,
  "stop": "[\\\"\\\\n\\\", \\\"###\\\"]",
  "stream": false,
  "temperature": 0.7,
  "tool_choice": null,
  "tools": [
    {}
  ],
  "top_p": 1,
  "user": "user_123"
}
```

<h3 id="processes-chat-requests-using-the-configured-task-chain.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|stackTrace|query|string|false|If provided the stacktraces will be added to the response.|
|body|body|[taskengine_OpenAIChatRequest](#schemataskengine_openaichatrequest)|true|none|
|chainID|path|string|true|The ID of the task chain to use.|

> Example responses

> 200 Response

> default Response

```json
{
  "error": {
    "code": "string",
    "message": "string",
    "param": "string",
    "type": "string"
  }
}
```

<h3 id="processes-chat-requests-using-the-configured-task-chain.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all configured external providers with pagination support.

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

<h3 id="lists-all-configured-external-providers-with-pagination-support.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|

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

<h3 id="lists-all-configured-external-providers-with-pagination-support.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_runtimestate_ProviderConfig](#schemaarray_runtimestate_providerconfig)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Removes provider configuration from the system.

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

<h3 id="removes-provider-configuration-from-the-system.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|providerType|path|string|true|The type of the provider to delete (e.g., 'openai', 'gemini').|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removes-provider-configuration-from-the-system.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves configuration details for a specific external provider.

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

<h3 id="retrieves-configuration-details-for-a-specific-external-provider.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|providerType|path|string|true|The type of the provider to delete (e.g., 'openai', 'gemini').|

> Example responses

> 200 Response

```json
{
  "APIKey": "string",
  "Type": "string"
}
```

<h3 id="retrieves-configuration-details-for-a-specific-external-provider.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[runtimestate_ProviderConfig](#schemaruntimestate_providerconfig)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves the current model download queue state.

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
If groups are enabled, models will only be downloaded to backends
that are associated with at least one group.

> Example responses

> 200 Response

```json
[
  {
    "createdAt": "2021-09-01T00:00:00Z",
    "id": "1234567890",
    "modelJob": {
      "model": "llama2:latest",
      "url": "http://ollama-prod.internal:11434"
    },
    "scheduledFor": 1630483200,
    "taskType": "model_download",
    "validUntil": 1630483200
  }
]
```

<h3 id="retrieves-the-current-model-download-queue-state.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_downloadservice_Job](#schemaarray_downloadservice_job)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Cancels an in-progress model download.

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

<h3 id="cancels-an-in-progress-model-download.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|model|query|string|false|The model name to cancel downloads for across all backends.|
|url|query|string|false|The base URL of a specific backend to cancel downloads on.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="cancels-an-in-progress-model-download.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Streams real-time download progress via Server-Sent Events (SSE).

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
  "error": {
    "code": "string",
    "message": "string",
    "param": "string",
    "type": "string"
  }
}
```

<h3 id="streams-real-time-download-progress-via-server-sent-events-(sse).-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Removes a model from the download queue.

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

<h3 id="removes-a-model-from-the-download-queue.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|model|path|string|true|The name of the model to remove from the queue (e.g., 'mistral:latest').|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="removes-a-model-from-the-download-queue.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves the current runtime state of all LLM backends.

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
backends and models that are assigned to the same group. Resources not in groups are ignored
for request processing even if they appear in this response.

> Example responses

> 200 Response

```json
[
  {
    "backend": {
      "baseUrl": "http://ollama-prod.internal:11434",
      "createdAt": "2023-11-15T14:30:45Z",
      "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
      "name": "ollama-production",
      "type": "ollama",
      "updatedAt": "2023-11-15T14:30:45Z"
    },
    "error": "connection timeout: context deadline exceeded",
    "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
    "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
    "name": "ollama-production",
    "pulledModels": {
      "canChat": true,
      "canEmbed": false,
      "canPrompt": true,
      "canStream": true,
      "contextLength": 4096,
      "details": {
        "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
        "family": "Mistral",
        "format": "gguf",
        "parameterSize": "7B",
        "parentModel": "mistral:7b",
        "quantizationLevel": "Q4_K_M"
      },
      "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
      "model": "mistral:instruct",
      "modifiedAt": "2023-11-15T14:30:45Z",
      "name": "Mistral 7B Instruct",
      "size": 4709611008
    }
  }
]
```

<h3 id="retrieves-the-current-runtime-state-of-all-llm-backends.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_statetype_BackendRuntimeState](#schemaarray_statetype_backendruntimestate)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists available task-chain hook types.

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

<h3 id="lists-available-task-chain-hook-types.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_string](#schemaarray_string)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Lists all task chain definitions with pagination.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/taskchains', headers = headers)

print(r.json())

```

`GET /taskchains`

Lists all task chain definitions with pagination.

<h3 id="lists-all-task-chain-definitions-with-pagination.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|limit|query|string|false|The maximum number of items to return per page.|
|cursor|query|string|false|An optional RFC3339Nano timestamp to fetch the next page of results.|

> Example responses

> 200 Response

```json
[
  {
    "debug": true,
    "description": "string",
    "id": "string",
    "tasks": {
      "compose": {
        "strategy": "string",
        "with_var": "string"
      },
      "description": "Validates user input meets quality requirements",
      "execute_config": {
        "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
        "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
        "model": "mistral:instruct",
        "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
        "pass_clients_tools": true,
        "provider": "ollama",
        "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
        "temperature": 0.7
      },
      "handler": "condition_key",
      "hook": {
        "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
        "name": "slack",
        "tool_name": "send_slack_notification"
      },
      "id": "validate_input",
      "input_var": "input",
      "output_template": "Hook result: {{.status}}",
      "print": "Validation result: {{.validate_input}}",
      "prompt_template": "Is this input valid? {{.input}}",
      "retry_on_failure": 2,
      "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
      "timeout": "30s",
      "transition": {
        "branches": {
          "goto": "positive_response",
          "operator": "equals",
          "when": "yes"
        },
        "on_failure": "error_handler"
      },
      "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
    },
    "token_limit": 0
  }
]
```

<h3 id="lists-all-task-chain-definitions-with-pagination.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[array_taskengine_TaskChainDefinition](#schemaarray_taskengine_taskchaindefinition)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Creates a new task chain definition.

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.post('/taskchains', headers = headers)

print(r.json())

```

`POST /taskchains`

Creates a new task chain definition.
The task chain is stored in the system's KV store for later execution.
Task chains define workflows with conditional branches, external hooks, and captured execution state.

> Body parameter

```json
{
  "debug": true,
  "description": "string",
  "id": "string",
  "tasks": {
    "compose": {
      "strategy": "string",
      "with_var": "string"
    },
    "description": "Validates user input meets quality requirements",
    "execute_config": {
      "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
      "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
      "model": "mistral:instruct",
      "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
      "pass_clients_tools": true,
      "provider": "ollama",
      "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
      "temperature": 0.7
    },
    "handler": "condition_key",
    "hook": {
      "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
      "name": "slack",
      "tool_name": "send_slack_notification"
    },
    "id": "validate_input",
    "input_var": "input",
    "output_template": "Hook result: {{.status}}",
    "print": "Validation result: {{.validate_input}}",
    "prompt_template": "Is this input valid? {{.input}}",
    "retry_on_failure": 2,
    "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
    "timeout": "30s",
    "transition": {
      "branches": {
        "goto": "positive_response",
        "operator": "equals",
        "when": "yes"
      },
      "on_failure": "error_handler"
    },
    "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
  },
  "token_limit": 0
}
```

<h3 id="creates-a-new-task-chain-definition.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[taskengine_TaskChainDefinition](#schemataskengine_taskchaindefinition)|true|none|

> Example responses

> 201 Response

```json
{
  "debug": true,
  "description": "string",
  "id": "string",
  "tasks": {
    "compose": {
      "strategy": "string",
      "with_var": "string"
    },
    "description": "Validates user input meets quality requirements",
    "execute_config": {
      "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
      "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
      "model": "mistral:instruct",
      "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
      "pass_clients_tools": true,
      "provider": "ollama",
      "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
      "temperature": 0.7
    },
    "handler": "condition_key",
    "hook": {
      "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
      "name": "slack",
      "tool_name": "send_slack_notification"
    },
    "id": "validate_input",
    "input_var": "input",
    "output_template": "Hook result: {{.status}}",
    "print": "Validation result: {{.validate_input}}",
    "prompt_template": "Is this input valid? {{.input}}",
    "retry_on_failure": 2,
    "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
    "timeout": "30s",
    "transition": {
      "branches": {
        "goto": "positive_response",
        "operator": "equals",
        "when": "yes"
      },
      "on_failure": "error_handler"
    },
    "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
  },
  "token_limit": 0
}
```

<h3 id="creates-a-new-task-chain-definition.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|Created|[taskengine_TaskChainDefinition](#schemataskengine_taskchaindefinition)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Deletes a task chain definition.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.delete('/taskchains/{id}', headers = headers)

print(r.json())

```

`DELETE /taskchains/{id}`

Deletes a task chain definition.

<h3 id="deletes-a-task-chain-definition.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier for the task chain.|

> Example responses

> 200 Response

```json
"string"
```

<h3 id="deletes-a-task-chain-definition.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|string|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Retrieves a specific task chain by ID.

> Code samples

```python
import requests
headers = {
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.get('/taskchains/{id}', headers = headers)

print(r.json())

```

`GET /taskchains/{id}`

Retrieves a specific task chain by ID.

<h3 id="retrieves-a-specific-task-chain-by-id.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The unique identifier for the task chain.|

> Example responses

> 200 Response

```json
{
  "debug": true,
  "description": "string",
  "id": "string",
  "tasks": {
    "compose": {
      "strategy": "string",
      "with_var": "string"
    },
    "description": "Validates user input meets quality requirements",
    "execute_config": {
      "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
      "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
      "model": "mistral:instruct",
      "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
      "pass_clients_tools": true,
      "provider": "ollama",
      "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
      "temperature": 0.7
    },
    "handler": "condition_key",
    "hook": {
      "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
      "name": "slack",
      "tool_name": "send_slack_notification"
    },
    "id": "validate_input",
    "input_var": "input",
    "output_template": "Hook result: {{.status}}",
    "print": "Validation result: {{.validate_input}}",
    "prompt_template": "Is this input valid? {{.input}}",
    "retry_on_failure": 2,
    "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
    "timeout": "30s",
    "transition": {
      "branches": {
        "goto": "positive_response",
        "operator": "equals",
        "when": "yes"
      },
      "on_failure": "error_handler"
    },
    "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
  },
  "token_limit": 0
}
```

<h3 id="retrieves-a-specific-task-chain-by-id.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[taskengine_TaskChainDefinition](#schemataskengine_taskchaindefinition)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Updates an existing task chain definition.

> Code samples

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json',
  'X-API-Key': 'API_KEY'
}

r = requests.put('/taskchains/{id}', headers = headers)

print(r.json())

```

`PUT /taskchains/{id}`

Updates an existing task chain definition.

> Body parameter

```json
{
  "debug": true,
  "description": "string",
  "id": "string",
  "tasks": {
    "compose": {
      "strategy": "string",
      "with_var": "string"
    },
    "description": "Validates user input meets quality requirements",
    "execute_config": {
      "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
      "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
      "model": "mistral:instruct",
      "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
      "pass_clients_tools": true,
      "provider": "ollama",
      "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
      "temperature": 0.7
    },
    "handler": "condition_key",
    "hook": {
      "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
      "name": "slack",
      "tool_name": "send_slack_notification"
    },
    "id": "validate_input",
    "input_var": "input",
    "output_template": "Hook result: {{.status}}",
    "print": "Validation result: {{.validate_input}}",
    "prompt_template": "Is this input valid? {{.input}}",
    "retry_on_failure": 2,
    "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
    "timeout": "30s",
    "transition": {
      "branches": {
        "goto": "positive_response",
        "operator": "equals",
        "when": "yes"
      },
      "on_failure": "error_handler"
    },
    "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
  },
  "token_limit": 0
}
```

<h3 id="updates-an-existing-task-chain-definition.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[taskengine_TaskChainDefinition](#schemataskengine_taskchaindefinition)|true|none|
|id|path|string|true|The unique identifier for the task chain.|

> Example responses

> 200 Response

```json
{
  "debug": true,
  "description": "string",
  "id": "string",
  "tasks": {
    "compose": {
      "strategy": "string",
      "with_var": "string"
    },
    "description": "Validates user input meets quality requirements",
    "execute_config": {
      "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
      "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
      "model": "mistral:instruct",
      "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
      "pass_clients_tools": true,
      "provider": "ollama",
      "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
      "temperature": 0.7
    },
    "handler": "condition_key",
    "hook": {
      "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
      "name": "slack",
      "tool_name": "send_slack_notification"
    },
    "id": "validate_input",
    "input_var": "input",
    "output_template": "Hook result: {{.status}}",
    "print": "Validation result: {{.validate_input}}",
    "prompt_template": "Is this input valid? {{.input}}",
    "retry_on_failure": 2,
    "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
    "timeout": "30s",
    "transition": {
      "branches": {
        "goto": "positive_response",
        "operator": "equals",
        "when": "yes"
      },
      "on_failure": "error_handler"
    },
    "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
  },
  "token_limit": 0
}
```

<h3 id="updates-an-existing-task-chain-definition.-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[taskengine_TaskChainDefinition](#schemataskengine_taskchaindefinition)|
|default|Default|Default error response|[ErrorResponse](#schemaerrorresponse)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
X-API-Key
</aside>

## Executes dynamic task-chain workflows.

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
Requests are routed ONLY to backends that have the requested model available in any shared group.
If groups are enabled, models and backends not assigned to any group will be completely ignored by the routing system.

> Body parameter

```json
{
  "chain": {
    "debug": true,
    "description": "string",
    "id": "string",
    "tasks": {
      "compose": {
        "strategy": "string",
        "with_var": "string"
      },
      "description": "Validates user input meets quality requirements",
      "execute_config": {
        "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
        "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
        "model": "mistral:instruct",
        "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
        "pass_clients_tools": true,
        "provider": "ollama",
        "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
        "temperature": 0.7
      },
      "handler": "condition_key",
      "hook": {
        "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
        "name": "slack",
        "tool_name": "send_slack_notification"
      },
      "id": "validate_input",
      "input_var": "input",
      "output_template": "Hook result: {{.status}}",
      "print": "Validation result: {{.validate_input}}",
      "prompt_template": "Is this input valid? {{.input}}",
      "retry_on_failure": 2,
      "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
      "timeout": "30s",
      "transition": {
        "branches": {
          "goto": "positive_response",
          "operator": "equals",
          "when": "yes"
        },
        "on_failure": "error_handler"
      },
      "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
    },
    "token_limit": 0
  },
  "input": null,
  "inputType": "string"
}
```

<h3 id="executes-dynamic-task-chain-workflows.-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[execapi_taskExecutionRequest](#schemaexecapi_taskexecutionrequest)|true|none|

> Example responses

> 200 Response

```json
{
  "output": null,
  "outputType": "string",
  "state": {
    "duration": 452000000,
    "error": {
      "error": "validation failed: input contains prohibited content"
    },
    "input": "This is a test input that needs validation",
    "inputType": "string",
    "inputVar": "input",
    "output": "valid",
    "outputType": "string",
    "taskHandler": "condition_key",
    "taskID": "validate_input",
    "transition": "valid_input"
  }
}
```

<h3 id="executes-dynamic-task-chain-workflows.-responses">Responses</h3>

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
  "error": {
    "code": "string",
    "message": "string",
    "param": "string",
    "type": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|error|object|true|none|none|
|» code|string|true|none|A specific error code identifier (e.g., 'invalid_parameter_value', 'unauthorized')|
|» message|string|true|none|A human-readable error message|
|» param|string¦null|false|none|The parameter that caused the error, if applicable|
|» type|string|true|none|The error type category (e.g., 'invalid_request_error', 'authentication_error')|

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
    "pulledModels": {
      "canChat": true,
      "canEmbed": false,
      "canPrompt": true,
      "canStream": true,
      "contextLength": 4096,
      "details": {
        "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
        "family": "Mistral",
        "format": "gguf",
        "parameterSize": "7B",
        "parentModel": "mistral:7b",
        "quantizationLevel": "Q4_K_M"
      },
      "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
      "model": "mistral:instruct",
      "modifiedAt": "2023-11-15T14:30:45Z",
      "name": "Mistral 7B Instruct",
      "size": 4709611008
    },
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
    "modelJob": {
      "model": "llama2:latest",
      "url": "http://ollama-prod.internal:11434"
    },
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

<h2 id="tocS_array_eventstore_Event">array_eventstore_Event</h2>
<!-- backwards compatibility -->
<a id="schemaarray_eventstore_event"></a>
<a id="schema_array_eventstore_Event"></a>
<a id="tocSarray_eventstore_event"></a>
<a id="tocsarray_eventstore_event"></a>

```json
[
  {
    "aggregate_id": "aggregate-uuid",
    "aggregate_type": "github.webhook",
    "created_at": "2023-01-01T00:00:00Z",
    "data": {},
    "event_source": "github.com",
    "event_type": "github.pull_request",
    "id": "event-uuid",
    "metadata": {},
    "nid": 1,
    "version": 1
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[eventstore_Event](#schemaeventstore_event)]|false|none|none|

<h2 id="tocS_array_functionstore_EventTrigger">array_functionstore_EventTrigger</h2>
<!-- backwards compatibility -->
<a id="schemaarray_functionstore_eventtrigger"></a>
<a id="schema_array_functionstore_EventTrigger"></a>
<a id="tocSarray_functionstore_eventtrigger"></a>
<a id="tocsarray_functionstore_eventtrigger"></a>

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "description": "Send a welcome email to a new user",
    "function": "new_user_created_event_handler",
    "listenFor": {
      "type": "contenox.user_created"
    },
    "name": "new_user_created",
    "type": "function",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[functionstore_EventTrigger](#schemafunctionstore_eventtrigger)]|false|none|none|

<h2 id="tocS_array_functionstore_Function">array_functionstore_Function</h2>
<!-- backwards compatibility -->
<a id="schemaarray_functionstore_function"></a>
<a id="schema_array_functionstore_Function"></a>
<a id="tocSarray_functionstore_function"></a>
<a id="tocsarray_functionstore_function"></a>

```json
[
  {
    "createdAt": "2023-11-15T14:30:45Z",
    "description": "string",
    "name": "send_welcome_email_event_handler",
    "script": "string",
    "scriptType": "goja",
    "updatedAt": "2023-11-15T14:30:45Z"
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[functionstore_Function](#schemafunctionstore_function)]|false|none|none|

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

<h2 id="tocS_array_runtimetypes_AffinityGroup">array_runtimetypes_AffinityGroup</h2>
<!-- backwards compatibility -->
<a id="schemaarray_runtimetypes_affinitygroup"></a>
<a id="schema_array_runtimetypes_AffinityGroup"></a>
<a id="tocSarray_runtimetypes_affinitygroup"></a>
<a id="tocsarray_runtimetypes_affinitygroup"></a>

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
|*anonymous*|[[runtimetypes_AffinityGroup](#schemaruntimetypes_affinitygroup)]|false|none|none|

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
    "headers": "Authorization:Bearer token,Content-Type:application/json",
    "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
    "name": "mailing-tools",
    "properties": {
      "in": "body",
      "name": "access_token",
      "value": null
    },
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
    "backend": {
      "baseUrl": "http://ollama-prod.internal:11434",
      "createdAt": "2023-11-15T14:30:45Z",
      "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
      "name": "ollama-production",
      "type": "ollama",
      "updatedAt": "2023-11-15T14:30:45Z"
    },
    "error": "connection timeout: context deadline exceeded",
    "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
    "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
    "name": "ollama-production",
    "pulledModels": {
      "canChat": true,
      "canEmbed": false,
      "canPrompt": true,
      "canStream": true,
      "contextLength": 4096,
      "details": {
        "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
        "family": "Mistral",
        "format": "gguf",
        "parameterSize": "7B",
        "parentModel": "mistral:7b",
        "quantizationLevel": "Q4_K_M"
      },
      "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
      "model": "mistral:instruct",
      "modifiedAt": "2023-11-15T14:30:45Z",
      "name": "Mistral 7B Instruct",
      "size": 4709611008
    }
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

<h2 id="tocS_array_taskengine_TaskChainDefinition">array_taskengine_TaskChainDefinition</h2>
<!-- backwards compatibility -->
<a id="schemaarray_taskengine_taskchaindefinition"></a>
<a id="schema_array_taskengine_TaskChainDefinition"></a>
<a id="tocSarray_taskengine_taskchaindefinition"></a>
<a id="tocsarray_taskengine_taskchaindefinition"></a>

```json
[
  {
    "debug": true,
    "description": "string",
    "id": "string",
    "tasks": {
      "compose": {
        "strategy": "string",
        "with_var": "string"
      },
      "description": "Validates user input meets quality requirements",
      "execute_config": {
        "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
        "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
        "model": "mistral:instruct",
        "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
        "pass_clients_tools": true,
        "provider": "ollama",
        "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
        "temperature": 0.7
      },
      "handler": "condition_key",
      "hook": {
        "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
        "name": "slack",
        "tool_name": "send_slack_notification"
      },
      "id": "validate_input",
      "input_var": "input",
      "output_template": "Hook result: {{.status}}",
      "print": "Validation result: {{.validate_input}}",
      "prompt_template": "Is this input valid? {{.input}}",
      "retry_on_failure": 2,
      "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
      "timeout": "30s",
      "transition": {
        "branches": {
          "goto": "positive_response",
          "operator": "equals",
          "when": "yes"
        },
        "on_failure": "error_handler"
      },
      "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
    },
    "token_limit": 0
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[taskengine_TaskChainDefinition](#schemataskengine_taskchaindefinition)]|false|none|none|

<h2 id="tocS_backendapi_OpenAICompatibleModelList">backendapi_OpenAICompatibleModelList</h2>
<!-- backwards compatibility -->
<a id="schemabackendapi_openaicompatiblemodellist"></a>
<a id="schema_backendapi_OpenAICompatibleModelList"></a>
<a id="tocSbackendapi_openaicompatiblemodellist"></a>
<a id="tocsbackendapi_openaicompatiblemodellist"></a>

```json
{
  "data": [
    {}
  ],
  "object": "list"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|data|[object]|true|none|none|
|object|string|true|none|none|

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
  "pulledModels": {
    "canChat": true,
    "canEmbed": false,
    "canPrompt": true,
    "canStream": true,
    "contextLength": 4096,
    "details": {
      "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
      "family": "Mistral",
      "format": "gguf",
      "parameterSize": "7B",
      "parentModel": "mistral:7b",
      "quantizationLevel": "Q4_K_M"
    },
    "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
    "model": "mistral:instruct",
    "modifiedAt": "2023-11-15T14:30:45Z",
    "name": "Mistral 7B Instruct",
    "size": 4709611008
  },
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
|pulledModels|[statetype_ModelPullStatus](#schemastatetype_modelpullstatus)|true|none|none|
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
  "pulledModels": {
    "canChat": true,
    "canEmbed": false,
    "canPrompt": true,
    "canStream": true,
    "contextLength": 4096,
    "details": {
      "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
      "family": "Mistral",
      "format": "gguf",
      "parameterSize": "7B",
      "parentModel": "mistral:7b",
      "quantizationLevel": "Q4_K_M"
    },
    "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
    "model": "mistral:instruct",
    "modifiedAt": "2023-11-15T14:30:45Z",
    "name": "Mistral 7B Instruct",
    "size": 4709611008
  },
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
|pulledModels|[statetype_ModelPullStatus](#schemastatetype_modelpullstatus)|true|none|none|
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
  "modelJob": {
    "model": "llama2:latest",
    "url": "http://ollama-prod.internal:11434"
  },
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
|modelJob|[runtimetypes_QueueItem](#schemaruntimetypes_queueitem)|true|none|none|
|scheduledFor|integer|true|none|none|
|taskType|string|true|none|none|
|validUntil|integer|true|none|none|

<h2 id="tocS_eventstore_Event">eventstore_Event</h2>
<!-- backwards compatibility -->
<a id="schemaeventstore_event"></a>
<a id="schema_eventstore_Event"></a>
<a id="tocSeventstore_event"></a>
<a id="tocseventstore_event"></a>

```json
{
  "aggregate_id": "aggregate-uuid",
  "aggregate_type": "github.webhook",
  "created_at": "2023-01-01T00:00:00Z",
  "data": {},
  "event_source": "github.com",
  "event_type": "github.pull_request",
  "id": "event-uuid",
  "metadata": {},
  "nid": 1,
  "version": 1
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|aggregate_id|string|true|none|none|
|aggregate_type|string|true|none|none|
|created_at|string(date-time)|true|none|none|
|data|string(json)|true|none|JSON-encoded string|
|event_source|string|true|none|none|
|event_type|string|true|none|none|
|id|string|true|none|none|
|metadata|string(json)|true|none|JSON-encoded string|
|nid|integer|true|none|none|
|version|integer|true|none|none|

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
  "chain": {
    "debug": true,
    "description": "string",
    "id": "string",
    "tasks": {
      "compose": {
        "strategy": "string",
        "with_var": "string"
      },
      "description": "Validates user input meets quality requirements",
      "execute_config": {
        "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
        "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
        "model": "mistral:instruct",
        "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
        "pass_clients_tools": true,
        "provider": "ollama",
        "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
        "temperature": 0.7
      },
      "handler": "condition_key",
      "hook": {
        "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
        "name": "slack",
        "tool_name": "send_slack_notification"
      },
      "id": "validate_input",
      "input_var": "input",
      "output_template": "Hook result: {{.status}}",
      "print": "Validation result: {{.validate_input}}",
      "prompt_template": "Is this input valid? {{.input}}",
      "retry_on_failure": 2,
      "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
      "timeout": "30s",
      "transition": {
        "branches": {
          "goto": "positive_response",
          "operator": "equals",
          "when": "yes"
        },
        "on_failure": "error_handler"
      },
      "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
    },
    "token_limit": 0
  },
  "input": null,
  "inputType": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|chain|[taskengine_TaskChainDefinition](#schemataskengine_taskchaindefinition)|true|none|none|
|input|[object](#schemaobject)|true|none|none|
|inputType|string|true|none|none|

<h2 id="tocS_execapi_taskExecutionResponse">execapi_taskExecutionResponse</h2>
<!-- backwards compatibility -->
<a id="schemaexecapi_taskexecutionresponse"></a>
<a id="schema_execapi_taskExecutionResponse"></a>
<a id="tocSexecapi_taskexecutionresponse"></a>
<a id="tocsexecapi_taskexecutionresponse"></a>

```json
{
  "output": null,
  "outputType": "string",
  "state": {
    "duration": 452000000,
    "error": {
      "error": "validation failed: input contains prohibited content"
    },
    "input": "This is a test input that needs validation",
    "inputType": "string",
    "inputVar": "input",
    "output": "valid",
    "outputType": "string",
    "taskHandler": "condition_key",
    "taskID": "validate_input",
    "transition": "valid_input"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|output|[object](#schemaobject)|true|none|none|
|outputType|string|true|none|none|
|state|[taskengine_CapturedStateUnit](#schemataskengine_capturedstateunit)|true|none|none|

<h2 id="tocS_execservice_SimpleExecutionResponse">execservice_SimpleExecutionResponse</h2>
<!-- backwards compatibility -->
<a id="schemaexecservice_simpleexecutionresponse"></a>
<a id="schema_execservice_SimpleExecutionResponse"></a>
<a id="tocSexecservice_simpleexecutionresponse"></a>
<a id="tocsexecservice_simpleexecutionresponse"></a>

```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "response": "I'm doing well, thank you!"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|true|none|none|
|response|string|true|none|none|

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

<h2 id="tocS_functionstore_EventTrigger">functionstore_EventTrigger</h2>
<!-- backwards compatibility -->
<a id="schemafunctionstore_eventtrigger"></a>
<a id="schema_functionstore_EventTrigger"></a>
<a id="tocSfunctionstore_eventtrigger"></a>
<a id="tocsfunctionstore_eventtrigger"></a>

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "Send a welcome email to a new user",
  "function": "new_user_created_event_handler",
  "listenFor": {
    "type": "contenox.user_created"
  },
  "name": "new_user_created",
  "type": "function",
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|createdAt|string(date-time)|true|none|Timestamps for creation and updates|
|description|string|true|none|A user-friendly description of what the trigger does.|
|function|string|true|none|The name of the function to execute when the event is received.|
|listenFor|[functionstore_Listener](#schemafunctionstore_listener)|true|none|none|
|name|string|true|none|A unique identifier for the trigger.|
|type|string|true|none|The type of the triggered action.|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_functionstore_Function">functionstore_Function</h2>
<!-- backwards compatibility -->
<a id="schemafunctionstore_function"></a>
<a id="schema_functionstore_Function"></a>
<a id="tocSfunctionstore_function"></a>
<a id="tocsfunctionstore_function"></a>

```json
{
  "createdAt": "2023-11-15T14:30:45Z",
  "description": "string",
  "name": "send_welcome_email_event_handler",
  "script": "string",
  "scriptType": "goja",
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|createdAt|string(date-time)|true|none|Timestamps for creation and updates|
|description|string|true|none|A user-friendly description of what the function does.|
|name|string|true|none|A unique identifier for the function.|
|script|string|true|none|The script code itself.|
|scriptType|string|true|none|The type of script to execute.|
|updatedAt|string(date-time)|true|none|none|

<h2 id="tocS_functionstore_Listener">functionstore_Listener</h2>
<!-- backwards compatibility -->
<a id="schemafunctionstore_listener"></a>
<a id="schema_functionstore_Listener"></a>
<a id="tocSfunctionstore_listener"></a>
<a id="tocsfunctionstore_listener"></a>

```json
{
  "type": "contenox.user_created"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|type|string|true|none|The event type to listen for.|

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
|APIKey|string|true|none|none|
|Type|string|true|none|none|

<h2 id="tocS_runtimetypes_AffinityGroup">runtimetypes_AffinityGroup</h2>
<!-- backwards compatibility -->
<a id="schemaruntimetypes_affinitygroup"></a>
<a id="schema_runtimetypes_AffinityGroup"></a>
<a id="tocSruntimetypes_affinitygroup"></a>
<a id="tocsruntimetypes_affinitygroup"></a>

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

<h2 id="tocS_runtimetypes_InjectionArg">runtimetypes_InjectionArg</h2>
<!-- backwards compatibility -->
<a id="schemaruntimetypes_injectionarg"></a>
<a id="schema_runtimetypes_InjectionArg"></a>
<a id="tocSruntimetypes_injectionarg"></a>
<a id="tocsruntimetypes_injectionarg"></a>

```json
{
  "in": "body",
  "name": "access_token",
  "value": null
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|in|string|true|none|none|
|name|string|true|none|none|
|value|[runtimetypes_any](#schemaruntimetypes_any)|true|none|none|

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

<h2 id="tocS_runtimetypes_QueueItem">runtimetypes_QueueItem</h2>
<!-- backwards compatibility -->
<a id="schemaruntimetypes_queueitem"></a>
<a id="schema_runtimetypes_QueueItem"></a>
<a id="tocSruntimetypes_queueitem"></a>
<a id="tocsruntimetypes_queueitem"></a>

```json
{
  "model": "llama2:latest",
  "url": "http://ollama-prod.internal:11434"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|model|string|true|none|none|
|url|string|true|none|none|

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
  "headers": "Authorization:Bearer token,Content-Type:application/json",
  "id": "h1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6",
  "name": "mailing-tools",
  "properties": {
    "in": "body",
    "name": "access_token",
    "value": null
  },
  "timeoutMs": 5000,
  "updatedAt": "2023-11-15T14:30:45Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|createdAt|string(date-time)|true|none|none|
|endpointUrl|string|true|none|none|
|headers|object|false|none|none|
|id|string|true|none|none|
|name|string|true|none|none|
|properties|[runtimetypes_InjectionArg](#schemaruntimetypes_injectionarg)|true|none|none|
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
  "backend": {
    "baseUrl": "http://ollama-prod.internal:11434",
    "createdAt": "2023-11-15T14:30:45Z",
    "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
    "name": "ollama-production",
    "type": "ollama",
    "updatedAt": "2023-11-15T14:30:45Z"
  },
  "error": "connection timeout: context deadline exceeded",
  "id": "b7d9e1a3-8f0c-4a7d-9b1e-2f3a4b5c6d7e",
  "models": "[\\\"mistral:instruct\\\", \\\"llama2:7b\\\", \\\"nomic-embed-text:latest\\\"]",
  "name": "ollama-production",
  "pulledModels": {
    "canChat": true,
    "canEmbed": false,
    "canPrompt": true,
    "canStream": true,
    "contextLength": 4096,
    "details": {
      "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
      "family": "Mistral",
      "format": "gguf",
      "parameterSize": "7B",
      "parentModel": "mistral:7b",
      "quantizationLevel": "Q4_K_M"
    },
    "digest": "sha256:9e3a6c0d3b5e7f8a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a",
    "model": "mistral:instruct",
    "modifiedAt": "2023-11-15T14:30:45Z",
    "name": "Mistral 7B Instruct",
    "size": 4709611008
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|backend|[runtimetypes_Backend](#schemaruntimetypes_backend)|true|none|none|
|error|string|false|none|Error stores a description of the last encountered error when<br>interacting with or reconciling this backend's state, if any.|
|id|string|true|none|none|
|models|[string]|true|none|none|
|name|string|true|none|none|
|pulledModels|[statetype_ModelPullStatus](#schemastatetype_modelpullstatus)|true|none|none|

<h2 id="tocS_statetype_ModelDetails">statetype_ModelDetails</h2>
<!-- backwards compatibility -->
<a id="schemastatetype_modeldetails"></a>
<a id="schema_statetype_ModelDetails"></a>
<a id="tocSstatetype_modeldetails"></a>
<a id="tocsstatetype_modeldetails"></a>

```json
{
  "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
  "family": "Mistral",
  "format": "gguf",
  "parameterSize": "7B",
  "parentModel": "mistral:7b",
  "quantizationLevel": "Q4_K_M"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|families|[string]|true|none|none|
|family|string|true|none|none|
|format|string|true|none|none|
|parameterSize|string|true|none|none|
|parentModel|string|true|none|none|
|quantizationLevel|string|true|none|none|

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
  "details": {
    "families": "[\\\"Mistral\\\", \\\"7B\\\"]",
    "family": "Mistral",
    "format": "gguf",
    "parameterSize": "7B",
    "parentModel": "mistral:7b",
    "quantizationLevel": "Q4_K_M"
  },
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
|details|[statetype_ModelDetails](#schemastatetype_modeldetails)|true|none|none|
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
  "error": {
    "error": "validation failed: input contains prohibited content"
  },
  "input": "This is a test input that needs validation",
  "inputType": "string",
  "inputVar": "input",
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
|duration|integer(nanoseconds)|true|none|in nanoseconds|
|error|[taskengine_ErrorResponse](#schemataskengine_errorresponse)|true|none|none|
|input|string|true|none|none|
|inputType|string|true|none|none|
|inputVar|string|true|none|Which variable was used as input|
|output|string|true|none|none|
|outputType|string|true|none|none|
|taskHandler|string|true|none|none|
|taskID|string|true|none|none|
|transition|string|true|none|none|

<h2 id="tocS_taskengine_ComposeTask">taskengine_ComposeTask</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_composetask"></a>
<a id="schema_taskengine_ComposeTask"></a>
<a id="tocStaskengine_composetask"></a>
<a id="tocstaskengine_composetask"></a>

```json
{
  "strategy": "string",
  "with_var": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|strategy|string|false|none|Strategy defines how values should be merged ("override", "merge_chat_histories", "append_string_to_chat_history").<br>Optional; defaults to "override" or "merge_chat_histories" if both output and WithVar values are ChatHistory.<br>"merge_chat_histories": If both output and WithVar values are ChatHistory,<br>appends the WithVar's Messages to the output's Messages.|
|with_var|string|false|none|Selects the variable to compose the current input with.|

<h2 id="tocS_taskengine_ErrorResponse">taskengine_ErrorResponse</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_errorresponse"></a>
<a id="schema_taskengine_ErrorResponse"></a>
<a id="tocStaskengine_errorresponse"></a>
<a id="tocstaskengine_errorresponse"></a>

```json
{
  "error": "validation failed: input contains prohibited content"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|error|string|true|none|none|

<h2 id="tocS_taskengine_HookCall">taskengine_HookCall</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_hookcall"></a>
<a id="schema_taskengine_HookCall"></a>
<a id="tocStaskengine_hookcall"></a>
<a id="tocstaskengine_hookcall"></a>

```json
{
  "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
  "name": "slack",
  "tool_name": "send_slack_notification"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|args|object|true|none|Args are key-value pairs to parameterize the hook call.<br>Example: {"to": "user@example.com", "subject": "Notification"}|
|name|string|true|none|Name is the registered hook-service (e.g., "send_email").|
|tool_name|string|true|none|ToolName is the name of the tool to invoke (e.g., "send_slack_notification").|

<h2 id="tocS_taskengine_LLMExecutionConfig">taskengine_LLMExecutionConfig</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_llmexecutionconfig"></a>
<a id="schema_taskengine_LLMExecutionConfig"></a>
<a id="tocStaskengine_llmexecutionconfig"></a>
<a id="tocstaskengine_llmexecutionconfig"></a>

```json
{
  "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
  "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
  "model": "mistral:instruct",
  "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
  "pass_clients_tools": true,
  "provider": "ollama",
  "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
  "temperature": 0.7
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|hide_tools|[string]|false|none|none|
|hooks|[string]|false|none|none|
|model|string|true|none|none|
|models|[string]|false|none|none|
|pass_clients_tools|boolean|true|none|none|
|provider|string|false|none|none|
|providers|[string]|false|none|none|
|temperature|number|false|none|none|

<h2 id="tocS_taskengine_OpenAIChatRequest">taskengine_OpenAIChatRequest</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_openaichatrequest"></a>
<a id="schema_taskengine_OpenAIChatRequest"></a>
<a id="tocStaskengine_openaichatrequest"></a>
<a id="tocstaskengine_openaichatrequest"></a>

```json
{
  "frequency_penalty": 0,
  "max_tokens": 512,
  "messages": {
    "content": "Hello, how are you?",
    "role": "user"
  },
  "model": "mistral:instruct",
  "n": 1,
  "presence_penalty": 0,
  "stop": "[\\\"\\\\n\\\", \\\"###\\\"]",
  "stream": false,
  "temperature": 0.7,
  "tool_choice": null,
  "tools": [
    {}
  ],
  "top_p": 1,
  "user": "user_123"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|frequency_penalty|number|false|none|none|
|max_tokens|integer|false|none|none|
|messages|[taskengine_OpenAIChatRequestMessage](#schemataskengine_openaichatrequestmessage)|true|none|none|
|model|string|true|none|none|
|n|integer|false|none|none|
|presence_penalty|number|false|none|none|
|stop|[string]|false|none|none|
|stream|boolean|false|none|none|
|temperature|number|false|none|none|
|tool_choice|any|false|none|Can be "none", "auto", or {"type": "function", "function": {"name": "my_function"}}|
|tools|[object]|false|none|none|
|top_p|number|false|none|none|
|user|string|false|none|none|

<h2 id="tocS_taskengine_OpenAIChatRequestMessage">taskengine_OpenAIChatRequestMessage</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_openaichatrequestmessage"></a>
<a id="schema_taskengine_OpenAIChatRequestMessage"></a>
<a id="tocStaskengine_openaichatrequestmessage"></a>
<a id="tocstaskengine_openaichatrequestmessage"></a>

```json
{
  "content": "Hello, how are you?",
  "role": "user"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|content|string|true|none|none|
|role|string|true|none|none|

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
  "tasks": {
    "compose": {
      "strategy": "string",
      "with_var": "string"
    },
    "description": "Validates user input meets quality requirements",
    "execute_config": {
      "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
      "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
      "model": "mistral:instruct",
      "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
      "pass_clients_tools": true,
      "provider": "ollama",
      "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
      "temperature": 0.7
    },
    "handler": "condition_key",
    "hook": {
      "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
      "name": "slack",
      "tool_name": "send_slack_notification"
    },
    "id": "validate_input",
    "input_var": "input",
    "output_template": "Hook result: {{.status}}",
    "print": "Validation result: {{.validate_input}}",
    "prompt_template": "Is this input valid? {{.input}}",
    "retry_on_failure": 2,
    "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
    "timeout": "30s",
    "transition": {
      "branches": {
        "goto": "positive_response",
        "operator": "equals",
        "when": "yes"
      },
      "on_failure": "error_handler"
    },
    "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
  },
  "token_limit": 0
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|debug|boolean|true|none|Enables capturing user input and output.|
|description|string|true|none|Description provides a human-readable summary of the chain's purpose.|
|id|string|true|none|ID uniquely identifies the chain.|
|tasks|[taskengine_TaskDefinition](#schemataskengine_taskdefinition)|true|none|none|
|token_limit|integer|true|none|TokenLimit is the token limit for the context window (used during execution).|

<h2 id="tocS_taskengine_TaskDefinition">taskengine_TaskDefinition</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_taskdefinition"></a>
<a id="schema_taskengine_TaskDefinition"></a>
<a id="tocStaskengine_taskdefinition"></a>
<a id="tocstaskengine_taskdefinition"></a>

```json
{
  "compose": {
    "strategy": "string",
    "with_var": "string"
  },
  "description": "Validates user input meets quality requirements",
  "execute_config": {
    "hide_tools": "[\\\"tool1\\\", \\\"hook_name1.tool1\\\"]",
    "hooks": "[\\\"slack_notification\\\", \\\"email_notification\\\"]",
    "model": "mistral:instruct",
    "models": "[\\\"gpt-4\\\", \\\"gpt-3.5-turbo\\\"]",
    "pass_clients_tools": true,
    "provider": "ollama",
    "providers": "[\\\"ollama\\\", \\\"openai\\\"]",
    "temperature": 0.7
  },
  "handler": "condition_key",
  "hook": {
    "args": "{\\\"channel\\\": \\\"#alerts\\\", \\\"message\\\": \\\"Task completed successfully\\\"}",
    "name": "slack",
    "tool_name": "send_slack_notification"
  },
  "id": "validate_input",
  "input_var": "input",
  "output_template": "Hook result: {{.status}}",
  "print": "Validation result: {{.validate_input}}",
  "prompt_template": "Is this input valid? {{.input}}",
  "retry_on_failure": 2,
  "system_instruction": "You are a quality control assistant. Respond only with 'valid' or 'invalid'.",
  "timeout": "30s",
  "transition": {
    "branches": {
      "goto": "positive_response",
      "operator": "equals",
      "when": "yes"
    },
    "on_failure": "error_handler"
  },
  "valid_conditions": "{\\\"valid\\\": true, \\\"invalid\\\": true}"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|compose|[taskengine_ComposeTask](#schemataskengine_composetask)|false|none|none|
|description|string|true|none|Description is a human-readable summary of what the task does.|
|execute_config|[taskengine_LLMExecutionConfig](#schemataskengine_llmexecutionconfig)|false|none|none|
|handler|string|true|none|Handler determines how the LLM output (or hook) will be interpreted.|
|hook|[taskengine_HookCall](#schemataskengine_hookcall)|false|none|none|
|id|string|true|none|ID uniquely identifies the task within the chain.|
|input_var|string|false|none|InputVar is the name of the variable to use as input for the task.<br>Example: "input" for the original input.<br>Each task stores its output in a variable named with it's task id.|
|output_template|string|false|none|OutputTemplate is an optional go template to format the output of a hook.<br>If specified, the hook's JSON output will be used as data for the template.<br>The final output of the task will be the rendered string.<br>Example: "The weather is {{.weather}} with a temperature of {{.temperature}}."|
|print|string|false|none|Print optionally formats the output for display/logging.<br>Supports template variables from previous task outputs.<br>Optional for all task types except Hook where it's rarely used.<br>Example: "The score is: {{.previous_output}}"|
|prompt_template|string|true|none|PromptTemplate is the text prompt sent to the LLM.<br>It's Required and only applicable for the raw_string type.<br>Supports template variables from previous task outputs.<br>Example: "Rate the quality from 1-10: {{.input}}"|
|retry_on_failure|integer|false|none|RetryOnFailure sets how many times to retry this task on failure.<br>Applies to all task types including Hooks.<br>Default: 0 (no retries)|
|system_instruction|string|false|none|SystemInstruction provides additional instructions to the LLM, if applicable system level will be used.|
|timeout|string|false|none|Timeout optionally sets a timeout for task execution.<br>Format: "10s", "2m", "1h" etc.<br>Optional for all task types.|
|transition|[taskengine_TaskTransition](#schemataskengine_tasktransition)|true|none|none|
|valid_conditions|object|false|none|ValidConditions defines allowed values for ConditionKey tasks.<br>Required for ConditionKey tasks, ignored for all other types.<br>Example: {"yes": true, "no": true} for a yes/no condition.|

<h2 id="tocS_taskengine_TaskTransition">taskengine_TaskTransition</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_tasktransition"></a>
<a id="schema_taskengine_TaskTransition"></a>
<a id="tocStaskengine_tasktransition"></a>
<a id="tocstaskengine_tasktransition"></a>

```json
{
  "branches": {
    "goto": "positive_response",
    "operator": "equals",
    "when": "yes"
  },
  "on_failure": "error_handler"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|branches|[taskengine_TransitionBranch](#schemataskengine_transitionbranch)|true|none|none|
|on_failure|string|true|none|OnFailure is the task ID to jump to in case of failure.|

<h2 id="tocS_taskengine_TransitionBranch">taskengine_TransitionBranch</h2>
<!-- backwards compatibility -->
<a id="schemataskengine_transitionbranch"></a>
<a id="schema_taskengine_TransitionBranch"></a>
<a id="tocStaskengine_transitionbranch"></a>
<a id="tocstaskengine_transitionbranch"></a>

```json
{
  "goto": "positive_response",
  "operator": "equals",
  "when": "yes"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|goto|string|true|none|Goto specifies the target task ID if this branch is taken.<br>Leave empty or use taskengine.TermEnd to end the chain.|
|operator|string|false|none|Operator defines how to compare the task's output to When.|
|when|string|true|none|When specifies the condition that must be met to follow this branch.<br>Format depends on the task type:<br>- For condition_key: exact string match<br>- For parse_number: numeric comparison (using Operator)|


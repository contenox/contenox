# contenox – Conversation Interface Engine

A modular platform for building context-aware agents, semantic search, and LLM-driven automation — all powered by a **conversation interface engine** that replaces traditional UIs with natural language interaction.

## 🧠 Key Features

- **Conversational UI Engine**: Replace buttons and menus with chat-based navigation and slash commands (`/echo`, `/summarize`, `/search`).
- **RAG-Powered Search & QA**: Ask questions and get answers grounded in your internal knowledge base.
- **Prompt Chain Automation**: Define repeatable actions using YAML-defined chains of prompts, hooks, and conditions.
- **Stateful Chat Sessions**: Maintain context across turns with role-based message history.
- **Command-Driven Interaction**: Use `/echo`, `/summarize`, and custom commands inside chat to trigger powerful actions.
- **Self-Hosted Infrastructure**: Full control over data and deployment — GDPR-ready and compliant with AI Act standards.
- **Modular Architecture**: Plug-and-play components for scalable, microservice-friendly design.
- **Open Core**: Built with Apache 2.0 licensing. Extendable, auditable, and enterprise-ready.

## 📦 What This Enables

You now have the foundation to support:

| Use Case | Description |
|---------|-------------|
| 📄 **Document QA Interface** | Users ask questions → system retrieves relevant documents → LLM synthesizes answer |
| 💬 **AI Chat Assistant** | Conversations maintain state, use commands like `/summarize`, `/help` |
| ⚙️ **Workflow Automation** | Chains define multi-step actions triggered by input or events |
| 🔍 **Semantic Search Engine** | Natural language queries over embedded documents |
| 🧩 **Extensible Hook System** | Add new capabilities easily via hooks (`/echo`, `/web`, `/clear`, etc.) |
| 🛠️ **Low-code Agent Development** | Build and deploy AI agents using YAML configuration only |

## 🔧 What's Under the Hood


contenox combines several technologies to deliver its features:

- **Core Logic**: The main backend service and LLM Gateway, built in **Go**, provides the primary API and orchestration.
- **User Interface**: Dashboards and user interactions are handled by a **React** frontend.
- **Auxiliary Services**: Data processing tasks like document mining and indexing are handled by separate services written in **Python**.
- **Language Models**: Core AI capabilities rely on **Ollama** backends.
- **Security**:
  - Authentication is handled via **JWT tokens**.
  - A **Backend-for-Frontend (BFF)** pattern helps the UI securely manage token lifecycles.
  - User management and permissions utilize a **custom Access Control system**.
- **Deployment**:
  - The system is designed to run **containerized** (e.g., using Docker).
  - Users are expected to provide external dependencies like **PostgreSQL**, **Valkey**, and **Vald**.
  - A `docker-compose.yml` file is provided for convenience, but operators typically deploy the contenox container image(s) directly and manage configuration externally.

## 📊 Current State (As of May 2025)

contenox is in **active development**. Building upon the foundational components, the recent focus on semantic search has integrated several key capabilities, resulting in the following established implementations:

* **Core Backend Services:** Modular Go services provide the backbone, including model management (`backendservice`), chat (`chatservice`), files (`fileservice`), users/access control (`userservice`, `accessservice`), **job dispatching (`dispatchservice`)**, **indexing (`indexservice`)**, and **backend pooling (`poolservice`)**.
* **Persistence & State:** Core data (users, files, jobs, etc.) is stored in **PostgreSQL** (`core/serverops/store`). **Vald** is integrated as the **vector store** for embeddings (`core/vectors`). **Valkey** (via `libkv`) is available for key-value storage/caching needs (not integrated yet). State synchronization for Ollama backends (`runtimestate`) is operational.
* **Document Ingestion & RAG Pipeline:** An initial Retrieval-Augmented Generation (RAG) pipeline is functional. **Python workers (`workers/`)** lease processing jobs (`dispatchapi`), fetch files (`filesapi`), parse/chunk documents, interact with core services for embedding generation, and **ingest vector embeddings into Vald** via the `indexservice`.
* **Semantic Search:** Basic semantic search capabilities over ingested documents are implemented. The `indexservice` handles embedding generation (using resolved LLM backends) and vector search queries against Vald.
* **LLM Integration & Management:** The system interfaces with Ollama backends (`libollama`, `modelprovider`). Logic for **resolving (`llmresolver`) the appropriate LLM backend/model** based on defined pools and scoring has been implemented.
* **Standalone Tokenizer Service:** Tokenization is handled by a dedicated **Go microservice (`tokenizer/`)** communicating via gRPC, optimizing core service resources and build times.
* **Foundational Libraries:** Shared Go libraries (`libs/`) provide robust, reusable components for database access (`libdb`), key-value stores (`libkv`), authentication (`libauth`), messaging (`libbus`), cryptography (`libcipher`), Ollama interaction (`libollama`), testing utilities (`libtestenv`), multi-step interactions and automation with Large Language Models (`taskengine`), and goroutine management (`libroutine`).
* **API & UI Structure:** A **React frontend (`frontend/`)** provides the user interface, built using Vite. Key features include routing (`src/config/routes.tsx`), foundational pages (Login, Chat, Admin views for Users, Backends, Files, **Semantic Search**, Server Jobs), and secure **JWT authentication managed via a Backend-for-Frontend (BFF)** pattern (`src/lib/AuthProvider.tsx`, `src/hooks/`). A basic UI component library (`packages/ui`) is used.
* **Basic Operations & CI:** The system is fully **containerized** (`Dockerfile.core`, `Dockerfile.tokenizer`, `Dockerfile.worker`, `compose.yaml`) with `make` targets for building, running (`make run`, `make ui-run`), and testing (`make api-test`). Basic Continuous Integration checks are operational.

## 🛠️ April Development Slice

-> Semantic Search
* [x] **UI-Search:** Develop a UI-Search page to demo semantic search.

  Steps needed:
    * [x] **Backend Pooling** Finalizing the implementation for grouping backends manageable pools/fleets assigning models to them.
    * [x] **Tokenizer Service Migration** Moving tokenizer logic into a dedicated service to optimize core service build times and resource usage.
    * [x] **Document Ingestion Pipeline:** Building the initial RAG pipeline, with Python workers, to parse and process documents from the filestore and ingest the embeddings into **vald**. *(Replaced opensearch with vald)*
    * [x] **LLM Resolver:** Improving the logic (`llmresolver`) for selecting the optimal backend instance and model for requests, via a scoring system and routing policies.
    * [x] **Fixing wiring:** Ensuring previously built features are fully integrated and functional E2E.
    * [x] **Cleaning:** Fix failing tests and get a basic CI running.

---
Notes from the devslice:
- *Rationale for Vald:* Vald was selected over OpenSearch due to its specialized focus on high-performance/scalable vector search, resulting in simpler integration as a core engine component compared to OpenSearch's broad feature set. Additional benefits included its suitable gRPC API, improved type-safety within the Go ecosystem, and faster spin-up times for development environments.

## 🛠️ May Development Slice

-> Documents QA
* [x] **Build UI-Documents QA Page:** This is about a UI page where a user can ask a question in a natural language format and gets a response with the most relevant documents.

  Steps needed:
    * [x] **Expose Prompt via a Service:** Create a service that can be used to execute a prompt, for workers to chunk text using semantic understanding.
    * [x] **Prompt Chains** Implement a prompt chain service that can be used to execute a sequence of prompts, for the QA page.
    * [x] **Improve Filesystem Performance:** Renaming files is currently slow, this is a nice to have task for this slice.
    * [x] **OpenAPI spec:** Review the endpoints and start establishing how to document APIs and how to serve the specifications.
    * [x] **Cleaning & wiring:** Ensure everything works as expected and tests are passing.

    ** Note: currently providing api documentation is not feasible.

## 🛠️ Current Development Slice

-> Packaging a Chat Application

* [ ] **Release processes**
* [ ] **Integrate Observability**
* [ ] **Prepare release infrastructure**
* [ ] **RAG-enhanced chat interface**
* [ ] **Chat with support for execution of tasks as commands**
* [ ] **Package a Persona-Chat Application**
* [ ] **Registration route for new Persona-Chat-Users**
* [ ] **Add API Rate Limiting middleware**

## ⚙️ Starting the Development Environment

### Prepare the Environment
  * Copy and edit `.env-example` into a new `.env` file with the proper configuration.
  * Install prerequisites: Docker, Docker Compose, Yarn, and Go.
### Build and Run the Backend Services
  * Run the following to build Docker images and start all services:
    ```bash
    make run
    ```
  * Use `make logs` to tail the backend logs if needed.
### Run the Frontend & UI
  * The backend includes a proxy (Backend-for-Frontend/BFF) to handle UI requests and authentication cookies correctly.
  * Start the UI development workflow, which builds UI components and runs the Vite dev server:
    ```bash
    make ui-run
    ```
  * Once Vite is running (you'll see its output in the terminal, often mentioning a port like 5173), **access the application in your browser via the main backend URL** (e.g., `http://localhost:8080` or as configured in your `.env` file or docker-compose port mappings).
  * **Important:** Do *not* use the local URL Vite might display (like `localhost:5173`), as login and other authenticated features will not work correctly through it due to how browser cookies are handled. Always access the UI through the backend's address during development. NOTE: Register
  as `admin@admin.com` for system privileges.

### API Tests Setup & Execution
  * Initialize the Python virtual environment and install API test dependencies:
    ```bash
    make api-init
    ```
  * Run your API tests via:
    ```bash
    make api-test
    ```

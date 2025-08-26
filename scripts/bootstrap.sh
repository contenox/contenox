#!/bin/bash

#
# This script automates the entire bootstrapping process for the contenox/runtime.
# It starts the necessary services, configures the backend and pools, and
# ensures the required models are downloaded and ready for use.
#
# Usage:
#   ./scripts/bootstrap.sh [embed_model] [task_model] [chat_model]
#   (e.g., ./bootstrap.sh nomic-embed-text:latest phi3:3.8b phi3:3.8b)
#
# Three models are required: embedding model, task model, and chat model
#

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Helper Functions ---
# Function to print messages
log() {
  echo "➡️  $1"
}

# Function to print success messages
success() {
  echo "✅ $1"
}

# Function to print error messages and exit
error_exit() {
  echo ""
  echo "❌ Error: $1"
  echo ""
  exit 1
}

# --- Configuration ---
API_BASE_URL="http://localhost:8081"
DEFAULT_EMBED_MODEL="nomic-embed-text:latest"
DEFAULT_TASK_MODEL="phi3:3.8b"
DEFAULT_CHAT_MODEL="phi3:3.8b"

# --- Process Command-Line Arguments ---
# If arguments provided, use them as models; otherwise use defaults
if [ $# -eq 3 ]; then
  log "Using command-line specified models:"
  log "  Embedding: $1"
  log "  Task: $2"
  log "  Chat: $3"
  EMBED_MODEL="$1"
  TASK_MODEL="$2"
  CHAT_MODEL="$3"
elif [ $# -eq 0 ]; then
  log "No models specified. Using default models:"
  log "  Embedding: ${DEFAULT_EMBED_MODEL}"
  log "  Task: ${DEFAULT_TASK_MODEL}"
  log "  Chat: ${DEFAULT_CHAT_MODEL}"
  EMBED_MODEL="${DEFAULT_EMBED_MODEL}"
  TASK_MODEL="${DEFAULT_TASK_MODEL}"
  CHAT_MODEL="${DEFAULT_CHAT_MODEL}"
else
  error_exit "Three models are required: embedding model, task model, and chat model"
fi

# Validate all models are non-empty
for model in "$EMBED_MODEL" "$TASK_MODEL" "$CHAT_MODEL"; do
  if [ -z "$model" ]; then
    error_exit "Empty model name detected. Please provide non-empty model names."
  fi
done

REQUIRED_MODELS=("$EMBED_MODEL" "$TASK_MODEL" "$CHAT_MODEL")

# --- Main Logic ---

# 1. Check for dependencies
log "Checking for required tools (docker, curl, jq)..."
for tool in docker curl jq; do
  if ! command -v $tool &> /dev/null; then
    error_exit "'$tool' is not installed. Please install it to continue."
  fi
done
success "All tools are available."

# 2. Start Docker services
log "Starting services with 'docker compose up -d'..."
docker compose up -d
success "Services are starting up."

# 3. Wait for the runtime API to be healthy
log "Waiting for the runtime API to become healthy..."
ATTEMPTS=0
MAX_ATTEMPTS=60 # Wait for up to 60 seconds
while ! curl -s -f "${API_BASE_URL}/health" > /dev/null; do
  ATTEMPTS=$((ATTEMPTS + 1))
  if [ $ATTEMPTS -ge $MAX_ATTEMPTS ]; then
    error_exit "Runtime API did not become healthy after $MAX_ATTEMPTS seconds. Please check the container logs with 'docker logs contenox-runtime-kernel'."
  fi
  sleep 1
done
success "Runtime API is healthy and responding."

log "Checking runtime version..."
VERSION=$(curl -s -f "${API_BASE_URL}/version" | jq -r .version)
success "Runtime version is $VERSION."

# 4. Register the 'local-ollama' backend if it doesn't exist
log "Checking for 'local-ollama' backend..."
response=$(curl -s -w "\n%{http_code}" "${API_BASE_URL}/backends")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [ "$http_code" -ne 200 ]; then
    error_exit "Failed to get backends. API returned status ${http_code}."
fi
BACKEND_ID=$(echo "$body" | jq -r '(. // []) | .[] | select(.name=="local-ollama") | .id')

if [ -z "$BACKEND_ID" ]; then
  log "Backend not found. Registering 'local-ollama'..."
  response=$(curl -s -w "\n%{http_code}" -X POST ${API_BASE_URL}/backends \
    -H "Content-Type: application/json" \
    -d '{
      "name": "local-ollama",
      "baseURL": "http://ollama:11434",
      "type": "ollama"
    }')
  http_code=$(echo "$response" | tail -n1)
  body=$(echo "$response" | sed '$d')

  if [ "$http_code" -ne 201 ]; then
    error_exit "Failed to register backend. API returned status ${http_code} with body: $body"
  fi
  BACKEND_ID=$(echo "$body" | jq -r '.id')

  if [ -z "$BACKEND_ID" ] || [ "$BACKEND_ID" == "null" ]; then
    error_exit "Failed to register backend (ID was null). Please check the runtime logs."
  fi
  success "Backend 'local-ollama' registered with ID: $BACKEND_ID"
else
  success "Backend 'local-ollama' already exists with ID: $BACKEND_ID"
fi

# 5. Assign backend to default pools if not already assigned
log "Assigning backend to default pools..."
# Pool 1: internal_tasks_pool
response=$(curl -s -w "\n%{http_code}" "${API_BASE_URL}/backend-associations/internal_tasks_pool/backends")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')
if [ "$http_code" -ne 200 ]; then
    error_exit "Failed to check task pool associations. API returned status ${http_code}."
fi
TASK_POOL_CHECK=$(echo "$body" | jq -r --arg BID "$BACKEND_ID" '(. // []) | .[] | select(.id==$BID) | .id')

if [ -z "$TASK_POOL_CHECK" ]; then
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE_URL}/backend-associations/internal_tasks_pool/backends/$BACKEND_ID")
  if [ "$http_code" -ne 201 ] && [ "$http_code" -ne 200 ]; then
      error_exit "Failed to assign backend to task pool. API returned status ${http_code}."
  fi
  success "Assigned backend to 'internal_tasks_pool'."
else
  success "Backend already assigned to 'internal_tasks_pool'."
fi

# Pool 2: internal_embed_pool
response=$(curl -s -w "\n%{http_code}" "${API_BASE_URL}/backend-associations/internal_embed_pool/backends")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')
if [ "$http_code" -ne 200 ]; then
    error_exit "Failed to check embed pool associations. API returned status ${http_code}."
fi
EMBED_POOL_CHECK=$(echo "$body" | jq -r --arg BID "$BACKEND_ID" '(. // []) | .[] | select(.id==$BID) | .id')

if [ -z "$EMBED_POOL_CHECK" ]; then
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE_URL}/backend-associations/internal_embed_pool/backends/$BACKEND_ID")
  if [ "$http_code" -ne 201 ] && [ "$http_code" -ne 200 ]; then
      error_exit "Failed to assign backend to embed pool. API returned status ${http_code}."
  fi
  success "Assigned backend to 'internal_embed_pool'."
else
  success "Backend already assigned to 'internal_embed_pool'."
fi

# Pool 3: internal_chat_pool
response=$(curl -s -w "\n%{http_code}" "${API_BASE_URL}/backend-associations/internal_chat_pool/backends")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')
if [ "$http_code" -ne 200 ]; then
    error_exit "Failed to check chat pool associations. API returned status ${http_code}."
fi
CHAT_POOL_CHECK=$(echo "$body" | jq -r --arg BID "$BACKEND_ID" '(. // []) | .[] | select(.id==$BID) | .id')

if [ -z "$CHAT_POOL_CHECK" ]; then
  http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE_URL}/backend-associations/internal_chat_pool/backends/$BACKEND_ID")
  # Treat 409 (Conflict) as success since it means the backend is already assigned
  if [ "$http_code" -ne 201 ] && [ "$http_code" -ne 200 ] && [ "$http_code" -ne 409 ]; then
      error_exit "Failed to assign backend to chat pool. API returned status ${http_code}."
  fi
  success "Assigned backend to 'internal_chat_pool'."
else
  success "Backend already assigned to 'internal_chat_pool'."
fi

# 6. Wait for models to be downloaded
log "Handing off to model download monitor..."
# Ensure the wait script is executable
log "Waiting for pool assignments to propagate..."
sleep 2
log "Handing off to model download monitor..."
./scripts/wait-for-models.sh "${REQUIRED_MODELS[@]}"

# Final success message
echo ""
echo "🎉 Bootstrap complete! Your contenox/runtime environment is ready to use."
echo "   Using models:"
echo "   - Embedding: ${EMBED_MODEL}"
echo "   - Task: ${TASK_MODEL}"
echo "   - Chat: ${CHAT_MODEL}"

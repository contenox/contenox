#!/bin/bash
#
# Ultra-minimal OpenAI Completion API bootstrap script
# ONLY creates the task chain needed for OpenAI compatibility
# Assumes EVERYTHING else is already configured (models, backends, affinity groups)
#
# Usage: ./openai-demo.sh <model-name> <chain-id>
# Example: ./openai-demo.sh phi3:3.8b demo
#

set -e

# Configuration - can be overridden by environment variables
API_BASE_URL="${API_BASE_URL:-http://localhost:8081}"
MODEL_NAME="$1"
CHAIN_ID="$2"

# Helper functions
log() { echo "➡️  $1"; }
success() { echo "✅ $1"; }
error_exit() { echo "❌ Error: $1"; exit 1; }

# Validate inputs
if [ -z "$MODEL_NAME" ] || [ -z "$CHAIN_ID" ]; then
    error_exit "Usage: $0 <model-name> <chain-id>"
    error_exit "Example: $0 phi3:3.8b demo"
    exit 1
fi

log "Creating OpenAI Completion API endpoint"
log "Using model: $MODEL_NAME"
log "Using chain ID: $CHAIN_ID"

# Check dependencies
for tool in curl jq; do
  if ! command -v $tool &> /dev/null; then
    error_exit "'$tool' is required but not installed"
  fi
done
success "Dependencies check passed"

# Wait for API to be ready
log "Waiting for API to become available..."
MAX_ATTEMPTS=30
for i in $(seq 1 $MAX_ATTEMPTS); do
  if curl -s -f "${API_BASE_URL}/health" >/dev/null 2>&1; then
    success "API is ready"
    break
  fi
  if [ $i -eq $MAX_ATTEMPTS ]; then
    error_exit "API did not become available after $MAX_ATTEMPTS seconds"
  fi
  sleep 1
done

# Create the task chain
log "Creating OpenAI-compatible task chain '$CHAIN_ID'..."
TASK_CHAIN=$(cat <<EOF
{
  "id": "$CHAIN_ID",
  "debug": true,
  "description": "OpenAI Completion API compatible chain",
  "token_limit": 4096,
  "tasks": [
    {
      "id": "main_task",
      "handler": "model_execution",
      "system_instruction": "You are a helpful AI assistant. Provide direct, concise answers.",
      "execute_config": {
        "model": "$MODEL_NAME"
      },
      "transition": {
        "branches": [
          { "operator": "default", "goto": "format_response" }
        ]
      }
    },
    {
      "id": "format_response",
      "handler": "convert_to_openai_chat_response",
      "transition": {
        "branches": [
          { "operator": "default", "goto": "end" }
        ]
      }
    }
  ]
}
EOF
)

# Create the task chain
curl -s -X POST "${API_BASE_URL}/taskchains" \
  -H "Content-Type: application/json" \
  -d "$TASK_CHAIN" >/dev/null

success "Task chain created successfully"

# Verify the endpoint is working
log "Verifying OpenAI endpoint..."
if curl -s -f "${API_BASE_URL}/openai/$CHAIN_ID/v1/models" >/dev/null; then
  success "OpenAI endpoint is ready!"
  echo ""
  echo "🎉 Setup complete! You can now use the OpenAI-compatible endpoint:"
  echo "   curl -X POST ${API_BASE_URL}/openai/$CHAIN_ID/v1/chat/completions \\"
  echo "        -H \"Content-Type: application/json\" \\"
  echo "        -d '{\"model\": \"$MODEL_NAME\", \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}]}'"
  echo ""
  echo "   Or with the OpenAI SDK:"
  echo "   client = OpenAI(base_url=\"${API_BASE_URL}/openai/$CHAIN_ID/v1\", api_key=\"anything\")"
  echo "   response = client.chat.completions.create(model=\"$MODEL_NAME\", messages=[{\"role\": \"user\", \"content\": \"Hello\"}])"
else
  error_exit "Endpoint verification failed - check if the model is downloaded and ready"
fi

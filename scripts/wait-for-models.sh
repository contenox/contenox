#!/bin/bash

#
# This script monitors the real-time download progress of specified models
# from the contenox/runtime API and displays a progress bar.
#
# Usage: ./wait-for-models.sh "model1:tag" "model2:tag" ...
#

# --- Configuration ---
# API endpoint for the download progress stream
PROGRESS_URL="http://localhost:8081/queue/inProgress"

# Timeout in seconds to prevent the script from running indefinitely
TIMEOUT_SECONDS=600

# --- Script Logic ---

# Check if model names are provided as arguments
if [ "$#" -eq 0 ]; then
  echo "Error: No model names provided."
  echo "Usage: $0 \"model1:tag\" \"model2:tag\" ..."
  exit 1
fi

# Use command-line arguments as the list of required models
REQUIRED_MODELS=("$@")

echo "⏳ Waiting for the following models to download (this may take 2-5 minutes):"
for model in "${REQUIRED_MODELS[@]}"; do
  echo "  - $model"
done
echo ""

# Use an associative array to track the download status of each model.
declare -A models_done
for model in "${REQUIRED_MODELS[@]}"; do
  models_done["$model"]=0
done

# Function to check if all models are downloaded
check_all_done() {
  for model in "${REQUIRED_MODELS[@]}"; do
    if [[ ${models_done["$model"]} -eq 0 ]]; then
      return 1 # Not all models are done
    fi
  done
  return 0 # All models are done
}

# Connect to the SSE stream and process progress events.
# The `timeout` command ensures the script exits if there's no activity.
timeout ${TIMEOUT_SECONDS}s curl -s -N "$PROGRESS_URL" | while read -r line; do
  # We only care for lines that start with "data:"
  if [[ $line == data:* ]]; then
    # Extract the JSON payload from the SSE message
    json_data=$(echo "$line" | sed 's/^data: //')

    # Parse model name, completed bytes, and total bytes
    model=$(echo "$json_data" | jq -r '.model')
    completed=$(echo "$json_data" | jq -r '.completed')
    total=$(echo "$json_data" | jq -r '.total')

    # Check if this update is for one of the models we are waiting for
    if [[ -v models_done["$model"] ]]; then

      # Calculate progress percentage
      if (( total > 0 )); then
        percent=$((completed * 100 / total))

        # Draw the progress bar, using \r to overwrite the same line
        bar_width=40
        completed_width=$((percent * bar_width / 100))

        # Create the progress bar string
        bar=$(printf "%-${bar_width}s" "$(printf '#%.0s' $(seq 1 $completed_width))")

        printf "  [%s] %3d%% - %s\r" "$bar" "$percent" "$model"

        # When a model is fully downloaded, mark it as done
        if (( percent >= 100 && models_done["$model"] == 0 )); then
          models_done["$model"]=1
          # Print a final "Done" line for the completed model
          printf "  [$(printf '#%.0s' $(seq 1 $bar_width))] 100%% - %s (Done)\n" "$model"
        fi
      fi

      # If all required models are downloaded, exit successfully
      if check_all_done; then
        echo -e "\n✅ All models are ready!"
        # This command kills the parent 'curl' process, which terminates the script
        kill $PPID
        exit 0
      fi
    fi
  fi
done

# Final check after the loop (in case of timeout or stream closing early)
if check_all_done; then
  echo -e "\n✅ Models were successfully downloaded."
else
  echo -e "\n\n⚠️  Warning: Script finished before all models were confirmed downloaded."
  echo "Please check the backend status manually by running: curl -s http://localhost:8081/backends | jq"
fi

#!/usr/bin/env bash

###############################################################################
# Load Environment Variables from .env
###############################################################################
# This block ensures we look in the same directory as this script for the .env.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"

if [ -f "$ENV_FILE" ]; then
  echo "Loading environment variables from .env..."
  set -o allexport
  source "$ENV_FILE"
  set +o allexport
else
  echo "Error: No .env file found in $SCRIPT_DIR. Please create one with your OPENAI_API_KEY."
  exit 1
fi

###############################################################################
# Safety Checks
###############################################################################

# 1. Exit immediately if a command exits with a non-zero status.
set -e

# 2. Check if OPENAI_API_KEY is set.
if [ -z "$OPENAI_API_KEY" ]; then
  echo "Error: OPENAI_API_KEY is not set or empty after loading .env."
  exit 1
fi

# 3. Check if 'jq' is installed.
if ! command -v jq &> /dev/null; then
  echo "Error: 'jq' is not installed or not in PATH."
  echo "Install 'jq' (e.g., sudo apt-get install jq) and retry."
  exit 1
fi

###############################################################################
# Step 1: List all files
###############################################################################

echo "Fetching the list of files from OpenAI..."

LIST_RESPONSE=$(curl -s \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  https://api.openai.com/v1/files)

# If the request fails or returns empty, handle it.
if [ -z "$LIST_RESPONSE" ]; then
  echo "Error: No response received from API. Aborting."
  exit 1
fi

# Extract file IDs using jq
FILE_IDS=$(echo "$LIST_RESPONSE" | jq -r '.data[].id' || true)

# If no files are found, exit early
if [ -z "$FILE_IDS" ]; then
  echo "No files found. Nothing to delete."
  exit 0
fi

echo "Found the following file IDs:"
echo "$FILE_IDS"
echo

###############################################################################
# Step 2: Confirm Deletion
###############################################################################

read -p "Are you sure you want to delete ALL these files? (y/N): " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
  echo "Aborting."
  exit 0
fi

###############################################################################
# Step 3: Delete each file
###############################################################################

echo "Proceeding to delete files..."
for file_id in $FILE_IDS; do
  echo "Deleting file with ID: $file_id"
  
  DELETE_RESPONSE=$(curl -s -X DELETE \
    -H "Authorization: Bearer $OPENAI_API_KEY" \
    https://api.openai.com/v1/files/"$file_id")

  # Optionally check the "deleted" field in the response
  deleted=$(echo "$DELETE_RESPONSE" | jq -r '.deleted' 2>/dev/null || true)
  if [ "$deleted" = "true" ]; then
    echo "Successfully deleted file $file_id."
  else
    echo "Warning: Could not confirm deletion for file $file_id."
    echo "Response was: $DELETE_RESPONSE"
  fi
  echo
done

echo "All deletions completed or attempted. Script finished."

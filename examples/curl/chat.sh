#!/usr/bin/env bash
# Chat, streaming, and embeddings against a running Polaris gateway.
#
#   export POLARIS_KEY=sk-...
#   ./chat.sh
set -euo pipefail

BASE="${POLARIS_BASE:-http://localhost:8080}"
KEY="${POLARIS_KEY:?set POLARIS_KEY to your Polaris API key}"
MODEL="${POLARIS_MODEL:-openai/gpt-4o}"

echo "== chat completion =="
curl -sS "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"Say hello in one word.\"}]}"
echo

echo "== streaming chat completion =="
curl -sS -N "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"$MODEL\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"Count to three.\"}]}"
echo

echo "== embeddings =="
curl -sS "$BASE/v1/embeddings" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/text-embedding-3-small","input":"the quick brown fox"}'
echo

echo "== list models =="
curl -sS "$BASE/v1/models" -H "Authorization: Bearer $KEY"
echo

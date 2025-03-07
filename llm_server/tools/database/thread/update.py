# tools/database/thread/update.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def update_thread(thread_id: str, model: str = None, provider: str = None, tokens_spent: int = None) -> dict:
    """
    Updates an existing thread by thread_id.
    Only non-None fields will be updated.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        # Return updated row
        "Prefer": "return=representation"
    }

    # Build a JSON payload with only the fields that are provided
    payload = {}
    if model is not None:
        payload["model"] = model
    if provider is not None:
        payload["provider"] = provider
    if tokens_spent is not None:
        payload["tokens_spent"] = tokens_spent

    # If there's nothing to update, you could either return early
    # or let PostgREST handle it
    if not payload:
        return {}

    url = f"{POSTGREST_BASE_URL}/threads?thread_id=eq.{thread_id}"

    # PostgREST uses PATCH for partial updates
    response = requests.patch(url, headers=headers, json=payload)
    response.raise_for_status()

    # Returns an array of updated rows
    return response.json()

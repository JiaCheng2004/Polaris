# tools/database/thread/create.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def create_thread(model: str, provider: str, tokens_spent: int = 0) -> dict:
    """
    Creates a new thread row in the database
    using PostgREST.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        # Optional: return the newly created row
        "Prefer": "return=representation"
    }

    payload = {
        "model": model,
        "provider": provider,
        "tokens_spent": tokens_spent
    }

    url = f"{POSTGREST_BASE_URL}/threads"

    response = requests.post(url, headers=headers, json=payload)
    response.raise_for_status()

    # If "Prefer: return=representation" is used,
    # PostgREST returns an array of the created rows
    return response.json()

# tools/database/thread/delete.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def delete_thread(thread_id: str) -> dict:
    """
    Deletes a thread (and cascades to messages) by thread_id.
    Returns the deleted thread record(s) if "Prefer" header is used.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}",
        # Return the deleted rows
        "Prefer": "return=representation"
    }

    url = f"{POSTGREST_BASE_URL}/threads?thread_id=eq.{thread_id}"
    response = requests.delete(url, headers=headers)
    response.raise_for_status()

    return response.json()  # Array of deleted row(s)

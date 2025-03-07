# tools/database/message/delete.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def delete_message(message_id: str) -> dict:
    """
    Deletes a single message by message_id.
    Returns the deleted message(s) if "Prefer" header is used.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}",
        "Prefer": "return=representation"
    }

    url = f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}"
    response = requests.delete(url, headers=headers)
    response.raise_for_status()

    return response.json()

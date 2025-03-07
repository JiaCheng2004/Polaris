# tools/database/message/update.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def update_message(message_id: str, role: str = None, content: dict = None, attachments: list = None) -> dict:
    """
    Partially updates a message by message_id.
    Only non-None fields will be updated.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"
    }

    payload = {}
    if role is not None:
        payload["role"] = role
    if content is not None:
        payload["content"] = content
    if attachments is not None:
        payload["attachments"] = attachments

    if not payload:
        return {}

    url = f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}"
    response = requests.patch(url, headers=headers, json=payload)
    response.raise_for_status()

    return response.json()

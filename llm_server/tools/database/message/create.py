# tools/database/message/create.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def create_message(thread_id: str, role: str, content: dict, attachments: list = None) -> dict:
    """
    Creates a new message under a given thread_id.
    `content` can be any JSON-serializable object (dict).
    `attachments` is a list of strings (file IDs).
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"
    }

    payload = {
        "thread_id": thread_id,
        "role": role,
        "content": content,
    }

    # Default attachments to empty list if not given
    payload["attachments"] = attachments if attachments is not None else []

    url = f"{POSTGREST_BASE_URL}/messages"

    response = requests.post(url, headers=headers, json=payload)
    response.raise_for_status()

    return response.json()

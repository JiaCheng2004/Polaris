# tools/database/message/read.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def get_message_by_id(message_id: str) -> dict:
    """
    Retrieves a single message by message_id.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}"
    }
    
    url = f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}"
    response = requests.get(url, headers=headers)
    response.raise_for_status()

    data = response.json()
    return data[0] if data else {}

def get_messages_by_thread(thread_id: str) -> list:
    """
    Retrieves all messages belonging to a given thread.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}"
    }
    
    url = f"{POSTGREST_BASE_URL}/messages?thread_id=eq.{thread_id}"
    response = requests.get(url, headers=headers)
    response.raise_for_status()

    return response.json()

def get_all_messages() -> list:
    """
    Retrieves all messages (use with caution in large datasets).
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}"
    }

    url = f"{POSTGREST_BASE_URL}/messages"
    response = requests.get(url, headers=headers)
    response.raise_for_status()

    return response.json()

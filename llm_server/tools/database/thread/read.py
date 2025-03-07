# tools/database/thread/read.py

import requests
from tools.config.load import POSTGREST_BASE_URL
from tools.database.auth.token import generate_token

def get_thread_by_id(thread_id: str) -> dict:
    """
    Retrieves a single thread by its ID.
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}"
    }

    url = f"{POSTGREST_BASE_URL}/threads?thread_id=eq.{thread_id}"
    response = requests.get(url, headers=headers)
    response.raise_for_status()

    # PostgREST returns rows as an array (possibly empty)
    data = response.json()
    return data[0] if data else {}

def get_all_threads() -> list:
    """
    Retrieves all threads (careful: this can be large).
    """
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}"
    }

    url = f"{POSTGREST_BASE_URL}/threads"
    response = requests.get(url, headers=headers)
    response.raise_for_status()

    return response.json()

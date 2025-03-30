# tools/database/thread/delete.py

import requests
from typing import Dict, Any, Optional, List
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def delete_thread(thread_id: str) -> bool:
    """
    Delete a thread from the database.
    
    Note: Due to cascade delete constraints in the database,
    this will also delete all associated messages and vectors
    for this thread.
    
    Args:
        thread_id (str): The UUID of the thread to delete
        
    Returns:
        bool: True if successfully deleted, False otherwise
        
    Raises:
        Exception: If the API request fails unexpectedly
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send DELETE request to remove the thread
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/threads?thread_id=eq.{thread_id}",
        headers=headers
    )
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # Thread not found, consider it already deleted
        return False
    else:
        error_message = f"Failed to delete thread: {response.status_code} - {response.text}"
        raise Exception(error_message) 
# tools/database/message/delete.py

import requests
from typing import Dict, Any, List, Optional, Union
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def delete_message(message_id: str) -> bool:
    """
    Delete a message from the database. This also deletes all file attachments via cascade delete.
    
    Args:
        message_id (str): The UUID of the message to delete
        
    Returns:
        bool: True if the message was successfully deleted
        
    Raises:
        Exception: If the message cannot be deleted or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send DELETE request
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}",
        headers=headers
    )
    
    # Check if the request was successful (204 No Content)
    if response.status_code == 204:
        return True
    else:
        error_message = f"Failed to delete message: {response.status_code} - {response.text}"
        raise Exception(error_message)

def delete_thread_messages(thread_id: str) -> bool:
    """
    Delete all messages from a thread.
    
    Args:
        thread_id (str): The UUID of the thread
        
    Returns:
        bool: True if the messages were successfully deleted
        
    Raises:
        Exception: If messages cannot be deleted or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send DELETE request
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/messages?thread_id=eq.{thread_id}",
        headers=headers
    )
    
    # Check if the request was successful (204 No Content)
    if response.status_code == 204:
        return True
    else:
        error_message = f"Failed to delete thread messages: {response.status_code} - {response.text}"
        raise Exception(error_message) 
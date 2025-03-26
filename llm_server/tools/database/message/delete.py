# tools/database/message/delete.py

import requests
from typing import Dict, Any, Optional, List
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def delete_message(message_id: str) -> bool:
    """
    Delete a message from the database.
    
    Note: Due to cascade delete constraints in the database,
    this will also delete all associated attachments for this message.
    
    Args:
        message_id (str): The UUID of the message to delete
        
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
    
    # Send DELETE request to remove the message
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}",
        headers=headers
    )
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # Message not found, consider it already deleted
        return False
    else:
        error_message = f"Failed to delete message: {response.status_code} - {response.text}"
        raise Exception(error_message)

def delete_thread_messages(thread_id: str) -> bool:
    """
    Delete all messages belonging to a specific thread.
    
    Args:
        thread_id (str): The UUID of the thread whose messages should be deleted
        
    Returns:
        bool: True if successfully deleted, False if no messages were found
        
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
    
    # Send DELETE request to remove all messages for the thread
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/messages?thread_id=eq.{thread_id}",
        headers=headers
    )
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # No messages found for this thread
        return False
    else:
        error_message = f"Failed to delete thread messages: {response.status_code} - {response.text}"
        raise Exception(error_message) 
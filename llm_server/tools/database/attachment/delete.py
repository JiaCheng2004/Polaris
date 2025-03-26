# tools/database/attachment/delete.py

import requests
from typing import Dict, Any, Optional, List
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def delete_attachment(file_id: str) -> bool:
    """
    Delete an attachment from the database.
    
    Args:
        file_id (str): The UUID of the attachment to delete
        
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
    
    # Send DELETE request to remove the attachment
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/attachments?file_id=eq.{file_id}",
        headers=headers
    )
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # Attachment not found, consider it already deleted
        return False
    else:
        error_message = f"Failed to delete attachment: {response.status_code} - {response.text}"
        raise Exception(error_message)

def delete_message_attachments(message_id: str) -> bool:
    """
    Delete all attachments belonging to a specific message.
    
    Args:
        message_id (str): The UUID of the message whose attachments should be deleted
        
    Returns:
        bool: True if successfully deleted, False if no attachments were found
        
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
    
    # Send DELETE request to remove all attachments for the message
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/attachments?message_id=eq.{message_id}",
        headers=headers
    )
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # No attachments found for this message
        return False
    else:
        error_message = f"Failed to delete message attachments: {response.status_code} - {response.text}"
        raise Exception(error_message) 
# tools/database/message/update.py

import requests
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL
from .create import attach_files_to_message
from .read import get_message_files

def update_message(
    message_id: str,
    content: Optional[Dict[str, Any]] = None,
    role: Optional[str] = None,
    author: Optional[Dict[str, Any]] = None,
    file_ids: Optional[List[str]] = None,
    replace_files: bool = False
) -> Dict[str, Any]:
    """
    Update an existing message.
    
    Args:
        message_id (str): The ID of the message to update
        content (Dict[str, Any], optional): New message content
        role (str, optional): New message role
        author (Dict[str, Any], optional): New message author data
        file_ids (List[str], optional): File IDs to attach to the message
        replace_files (bool, optional): If True, replace existing file attachments; 
                                       if False, add to existing. Defaults to False.
        
    Returns:
        Dict[str, Any]: The updated message data
        
    Raises:
        Exception: If the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the updated resource
    }
    
    # Build update data from non-None parameters
    update_data = {}
    if content is not None:
        update_data["content"] = content
    if role is not None:
        update_data["role"] = role
    if author is not None:
        update_data["author"] = author
    
    # Only proceed with API call if there's data to update
    if update_data:
        # Send PATCH request to update the message
        response = requests.patch(
            f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}",
            headers=headers,
            json=update_data
        )
        
        # Check if the request was successful
        if response.status_code != 200:
            error_message = f"Failed to update message: {response.status_code} - {response.text}"
            raise Exception(error_message)
    
    # Handle file attachments if provided
    if file_ids is not None and len(file_ids) > 0:
        if replace_files:
            # Get existing file attachments
            existing_files = get_message_files(message_id, token)
            existing_file_ids = [file["file_id"] for file in existing_files]
            
            # Delete existing file attachments
            if existing_file_ids:
                delete_message_files(message_id, existing_file_ids, token)
                
        # Attach new files
        attach_files_to_message(message_id, file_ids, token)
    
    # Get the updated message with its attached files
    return get_message(message_id)

def delete_message_files(message_id: str, file_ids: List[str], token: Optional[str] = None) -> bool:
    """
    Delete specific file attachments from a message.
    
    Args:
        message_id (str): The ID of the message
        file_ids (List[str]): List of file IDs to detach
        token (str, optional): Auth token. If None, a new one will be generated.
        
    Returns:
        bool: True if successful
        
    Raises:
        Exception: If the API request fails
    """
    # Generate auth token for PostgREST if not provided
    if token is None:
        token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Build the file_id filter for the query
    file_ids_filter = ",".join([f"eq.{file_id}" for file_id in file_ids])
    
    # Send DELETE request to remove the message_files records
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/message_files?message_id=eq.{message_id}&file_id=or.({file_ids_filter})",
        headers=headers
    )
    
    # Check if the request was successful
    if response.status_code == 204:  # No Content
        return True
    else:
        error_message = f"Failed to delete message files: {response.status_code} - {response.text}"
        raise Exception(error_message)

# Import at the end to avoid circular imports
from .read import get_message 
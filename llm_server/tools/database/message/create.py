# tools/database/message/create.py

import requests
import json
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def create_message(
    thread_id: str,
    role: str,
    content: Dict[str, Any],
    author: Dict[str, Any],
    file_ids: Optional[List[str]] = None
) -> Dict[str, Any]:
    """
    Create a new message in the database.
    
    Args:
        thread_id (str): The ID of the thread this message belongs to (prefixed with 'thread-')
        role (str): The role of the message sender (e.g., "user", "assistant", "system")
        content (Dict[str, Any]): JSON data containing the message content
            Example: {"type": "text", "text": "Hello, how can I help you?"}
        author (Dict[str, Any]): JSON data describing the author of the message
        file_ids (List[str], optional): List of file IDs to attach to this message
        
    Returns:
        Dict[str, Any]: The created message data including message_id (prefixed with 'message-')
        
    Raises:
        Exception: If the API request fails
    """
    # Prepare the request data
    message_data = {
        "thread_id": thread_id,
        "role": role,
        "content": content,
        "author": author
    }
    
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the created resource
    }
    
    # Send POST request to create the message
    response = requests.post(
        f"{POSTGREST_BASE_URL}/messages",
        headers=headers,
        json=message_data
    )
    
    # Check if the request was successful
    if response.status_code == 201:
        created_message = response.json()[0]  # PostgREST returns array with single item
        
        # If file_ids are provided, attach them to the message
        if file_ids and len(file_ids) > 0:
            message_id = created_message["message_id"]
            attach_files_to_message(message_id, file_ids, token)
            
        return created_message
    else:
        error_message = f"Failed to create message: {response.status_code} - {response.text}"
        raise Exception(error_message)

def attach_files_to_message(message_id: str, file_ids: List[str], token: Optional[str] = None) -> List[Dict[str, Any]]:
    """
    Attach files to an existing message.
    
    Args:
        message_id (str): The ID of the message to attach files to
        file_ids (List[str]): List of file IDs to attach
        token (str, optional): Auth token. If None, a new one will be generated.
        
    Returns:
        List[Dict[str, Any]]: The created message_files records
        
    Raises:
        Exception: If the API request fails
    """
    # Generate auth token for PostgREST if not provided
    if token is None:
        token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the created resources
    }
    
    # Prepare batch of message_files records
    message_files_data = [
        {"message_id": message_id, "file_id": file_id}
        for file_id in file_ids
    ]
    
    # Send POST request to create the message_files records
    response = requests.post(
        f"{POSTGREST_BASE_URL}/message_files",
        headers=headers,
        json=message_files_data
    )
    
    # Check if the request was successful
    if response.status_code == 201:
        return response.json()
    else:
        error_message = f"Failed to attach files to message: {response.status_code} - {response.text}"
        raise Exception(error_message) 
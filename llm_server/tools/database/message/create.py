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
    purpose: str,
    author: Dict[str, Any]
) -> Dict[str, Any]:
    """
    Create a new message in the database.
    
    Args:
        thread_id (str): The ID of the thread this message belongs to (prefixed with 'thread-')
        role (str): The role of the message sender (e.g., "user", "assistant", "system")
        content (Dict[str, Any]): JSON data containing the message content
            Example: {"type": "text", "text": "Hello, how can I help you?"}
        purpose (str): The purpose of the message (e.g., "reply", "summary", "annotation")
        author (Dict[str, Any]): JSON data describing the author of the message
        
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
        "purpose": purpose,
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
        return response.json()[0]  # PostgREST returns array with single item
    else:
        error_message = f"Failed to create message: {response.status_code} - {response.text}"
        raise Exception(error_message) 
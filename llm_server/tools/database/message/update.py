# tools/database/message/update.py

import requests
from typing import Dict, Any, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def update_message(
    message_id: str, 
    update_data: Dict[str, Any]
) -> Dict[str, Any]:
    """
    Update an existing message in the database.
    
    Args:
        message_id (str): The UUID of the message to update
        update_data (Dict[str, Any]): Dictionary containing the fields to update
            Possible fields:
            - content (Dict): JSON data containing the message content
            - purpose (str): The purpose of the message
            - role (str): The role of the message sender
            
    Returns:
        Dict[str, Any]: The updated message data
        
    Raises:
        Exception: If the message is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the updated resource
    }
    
    # Send PATCH request to update the message
    response = requests.patch(
        f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}",
        headers=headers,
        json=update_data
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Message not found with ID: {message_id}")
    else:
        error_message = f"Failed to update message: {response.status_code} - {response.text}"
        raise Exception(error_message) 
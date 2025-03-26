# tools/database/message/read.py

import requests
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def get_message(message_id: str) -> Dict[str, Any]:
    """
    Retrieve a specific message by its ID.
    
    Args:
        message_id (str): The UUID of the message to retrieve
        
    Returns:
        Dict[str, Any]: The message data
        
    Raises:
        Exception: If the message is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send GET request to retrieve the message
    response = requests.get(
        f"{POSTGREST_BASE_URL}/messages?message_id=eq.{message_id}",
        headers=headers
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Message not found with ID: {message_id}")
    else:
        error_message = f"Failed to retrieve message: {response.status_code} - {response.text}"
        raise Exception(error_message)

def list_messages(
    thread_id: Optional[str] = None,
    limit: int = 100, 
    offset: int = 0,
    order_by: str = "created_at.asc",
    role: Optional[str] = None,
    purpose: Optional[str] = None
) -> List[Dict[str, Any]]:
    """
    List messages with optional filtering and sorting.
    
    Args:
        thread_id (str, optional): Filter by thread ID. Defaults to None.
        limit (int, optional): Maximum number of messages to return. Defaults to 100.
        offset (int, optional): Number of messages to skip. Defaults to 0.
        order_by (str, optional): Field and direction to sort by. Defaults to "created_at.asc".
        role (str, optional): Filter by message role (e.g., "user", "assistant"). Defaults to None.
        purpose (str, optional): Filter by message purpose. Defaults to None.
            
    Returns:
        List[Dict[str, Any]]: List of message objects
        
    Raises:
        Exception: If the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Build the base URL
    url = f"{POSTGREST_BASE_URL}/messages"
    
    # Add query parameters
    params = {
        "limit": limit,
        "offset": offset,
        "order": order_by
    }
    
    # Add filter for thread_id if provided
    if thread_id:
        params["thread_id"] = f"eq.{thread_id}"
        
    # Add filter for role if provided
    if role:
        params["role"] = f"eq.{role}"
        
    # Add filter for purpose if provided
    if purpose:
        params["purpose"] = f"eq.{purpose}"
    
    # Send GET request to retrieve messages
    response = requests.get(url, headers=headers, params=params)
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to list messages: {response.status_code} - {response.text}"
        raise Exception(error_message)

def get_thread_conversation(
    thread_id: str,
    include_system: bool = False,
    limit: int = 100,
    newest_first: bool = False
) -> List[Dict[str, Any]]:
    """
    Get a conversation from a thread, with messages ordered by timestamp.
    
    Args:
        thread_id (str): The UUID of the thread to retrieve messages from
        include_system (bool, optional): Whether to include system messages. Defaults to False.
        limit (int, optional): Maximum number of messages to return. Defaults to 100.
        newest_first (bool, optional): If True, return newest messages first. Defaults to False.
            
    Returns:
        List[Dict[str, Any]]: List of message objects in conversation order
        
    Raises:
        Exception: If the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Build the base URL
    url = f"{POSTGREST_BASE_URL}/messages"
    
    # Set ordering (newest or oldest first)
    order_direction = "desc" if newest_first else "asc"
    
    # Add query parameters
    params = {
        "thread_id": f"eq.{thread_id}",
        "limit": limit,
        "order": f"created_at.{order_direction}"
    }
    
    # Add filter to exclude system messages if needed
    if not include_system:
        params["role"] = "neq.system"
    
    # Send GET request to retrieve messages
    response = requests.get(url, headers=headers, params=params)
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to get thread conversation: {response.status_code} - {response.text}"
        raise Exception(error_message) 
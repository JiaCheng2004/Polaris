# tools/database/vector/delete.py

import requests
from typing import Dict, Any, Optional, List
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def delete_vector(vector_id: str) -> bool:
    """
    Delete a vector from the database.
    
    Args:
        vector_id (str): The UUID of the vector to delete
        
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
    
    # Send DELETE request to remove the vector
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/vector_store?vector_id=eq.{vector_id}",
        headers=headers
    )
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # Vector not found, consider it already deleted
        return False
    else:
        error_message = f"Failed to delete vector: {response.status_code} - {response.text}"
        raise Exception(error_message)

def delete_thread_vectors(
    thread_id: str, 
    namespace: Optional[str] = None
) -> bool:
    """
    Delete all vectors belonging to a specific thread.
    
    Args:
        thread_id (str): The UUID of the thread whose vectors should be deleted
        namespace (str, optional): Filter by namespace. Defaults to None.
        
    Returns:
        bool: True if successfully deleted, False if no vectors were found
        
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
    
    # Build the URL with filters
    url = f"{POSTGREST_BASE_URL}/vector_store?thread_id=eq.{thread_id}"
    if namespace:
        url += f"&metadata->>'namespace'=eq.{namespace}"
    
    # Send DELETE request to remove vectors for the thread
    response = requests.delete(url, headers=headers)
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # No vectors found for this thread
        return False
    else:
        error_message = f"Failed to delete thread vectors: {response.status_code} - {response.text}"
        raise Exception(error_message)

def delete_message_vectors(message_id: str) -> bool:
    """
    Delete all vectors belonging to a specific message.
    
    Args:
        message_id (str): The UUID of the message whose vectors should be deleted
        
    Returns:
        bool: True if successfully deleted, False if no vectors were found
        
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
    
    # Send DELETE request to remove vectors for the message
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/vector_store?metadata->>'message_id'=eq.{message_id}",
        headers=headers
    )
    
    # Check if the request was successful
    # Status 204 indicates success with no content returned
    if response.status_code == 204:
        return True
    elif response.status_code == 404:
        # No vectors found for this message
        return False
    else:
        error_message = f"Failed to delete message vectors: {response.status_code} - {response.text}"
        raise Exception(error_message) 
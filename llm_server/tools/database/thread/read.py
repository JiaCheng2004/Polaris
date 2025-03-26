# tools/database/thread/read.py

import requests
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def get_thread(thread_id: str) -> Dict[str, Any]:
    """
    Retrieve a specific thread by its ID.
    
    Args:
        thread_id (str): The UUID of the thread to retrieve
        
    Returns:
        Dict[str, Any]: The thread data
        
    Raises:
        Exception: If the thread is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send GET request to retrieve the thread
    response = requests.get(
        f"{POSTGREST_BASE_URL}/threads?thread_id=eq.{thread_id}",
        headers=headers
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Thread not found with ID: {thread_id}")
    else:
        error_message = f"Failed to retrieve thread: {response.status_code} - {response.text}"
        raise Exception(error_message)

def list_threads(
    limit: int = 100, 
    offset: int = 0,
    order_by: str = "created_at.desc",
    filters: Optional[Dict[str, Any]] = None
) -> List[Dict[str, Any]]:
    """
    List threads with optional filtering and sorting.
    
    Args:
        limit (int, optional): Maximum number of threads to return. Defaults to 100.
        offset (int, optional): Number of threads to skip. Defaults to 0.
        order_by (str, optional): Field and direction to sort by. Defaults to "created_at.desc".
        filters (Dict[str, Any], optional): Dictionary of filter conditions. Defaults to None.
            Example: {"purpose": "web app", "provider": "openai"}
            
    Returns:
        List[Dict[str, Any]]: List of thread objects
        
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
    url = f"{POSTGREST_BASE_URL}/threads"
    
    # Add query parameters
    params = {
        "limit": limit,
        "offset": offset,
        "order": order_by
    }
    
    # Add filter conditions if provided
    if filters:
        for key, value in filters.items():
            if isinstance(value, str):
                params[f"{key}"] = f"eq.{value}"
            elif isinstance(value, (int, float)):
                params[f"{key}"] = f"eq.{value}"
    
    # Send GET request to retrieve threads
    response = requests.get(url, headers=headers, params=params)
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to list threads: {response.status_code} - {response.text}"
        raise Exception(error_message) 
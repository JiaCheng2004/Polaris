# tools/database/thread/update.py

import requests
from typing import Dict, Any, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def update_thread(
    thread_id: str, 
    update_data: Dict[str, Any]
) -> Dict[str, Any]:
    """
    Update an existing thread in the database.
    
    Args:
        thread_id (str): The UUID of the thread to update
        update_data (Dict[str, Any]): Dictionary containing the fields to update
            Possible fields:
            - model (str): The LLM model being used
            - provider (str): The provider of the model
            - tokens_spent (int): Token count
            - cost (float): Monetary cost
            - purpose (str): The purpose of the thread
            - author (Dict): JSON data describing the user(s)
        
    Returns:
        Dict[str, Any]: The updated thread data
        
    Raises:
        Exception: If the thread is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the updated resource
    }
    
    # Send PATCH request to update the thread
    response = requests.patch(
        f"{POSTGREST_BASE_URL}/threads?thread_id=eq.{thread_id}",
        headers=headers,
        json=update_data
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Thread not found with ID: {thread_id}")
    else:
        error_message = f"Failed to update thread: {response.status_code} - {response.text}"
        raise Exception(error_message)

def increment_thread_usage(
    thread_id: str,
    additional_tokens: int = 0,
    additional_cost: float = 0.0
) -> Dict[str, Any]:
    """
    Increment the token count and cost for a thread.
    
    Args:
        thread_id (str): The UUID of the thread to update
        additional_tokens (int, optional): Additional tokens to add. Defaults to 0.
        additional_cost (float, optional): Additional cost to add. Defaults to 0.0.
        
    Returns:
        Dict[str, Any]: The updated thread data
        
    Raises:
        Exception: If the thread is not found or the API request fails
    """
    # First, get the current thread to calculate new values
    token = generate_token()
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Get current thread data
    response = requests.get(
        f"{POSTGREST_BASE_URL}/threads?thread_id=eq.{thread_id}",
        headers=headers
    )
    
    if response.status_code != 200 or not response.json():
        raise Exception(f"Thread not found with ID: {thread_id}")
    
    current_thread = response.json()[0]
    current_tokens = current_thread.get("tokens_spent", 0)
    current_cost = current_thread.get("cost", 0.0)
    
    # Calculate new values
    new_tokens = current_tokens + additional_tokens
    new_cost = current_cost + additional_cost
    
    # Update the thread with new values
    update_data = {
        "tokens_spent": new_tokens,
        "cost": new_cost
    }
    
    # Use the update_thread function to perform the update
    return update_thread(thread_id, update_data) 
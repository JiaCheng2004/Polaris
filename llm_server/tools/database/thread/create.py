# tools/database/thread/create.py

import requests
import json
from typing import Dict, Any, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def create_thread(
    model: str,
    provider: str,
    purpose: str,
    author: Dict[str, Any],
    tokens_spent: int = 0,
    cost: float = 0.0
) -> Dict[str, Any]:
    """
    Create a new thread in the database.
    
    Args:
        model (str): The LLM model being used (e.g., "gpt-4", "claude-3")
        provider (str): The provider of the model (e.g., "openai", "anthropic")
        purpose (str): The purpose of the thread (e.g., "discord bot", "web app")
        author (Dict[str, Any]): JSON data describing the user(s)
        tokens_spent (int, optional): Initial token count. Defaults to 0.
        cost (float, optional): Initial monetary cost. Defaults to 0.0.
        
    Returns:
        Dict[str, Any]: The created thread data including thread_id (prefixed with 'thread-')
        
    Raises:
        Exception: If the API request fails
    """
    # Prepare the request data
    thread_data = {
        "model": model,
        "provider": provider,
        "tokens_spent": tokens_spent,
        "cost": cost,
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
    
    # Send POST request to create the thread
    response = requests.post(
        f"{POSTGREST_BASE_URL}/threads",
        headers=headers,
        json=thread_data
    )
    
    # Check if the request was successful
    if response.status_code == 201:
        return response.json()[0]  # PostgREST returns array with single item
    else:
        error_message = f"Failed to create thread: {response.status_code} - {response.text}"
        raise Exception(error_message) 
# tools/database/vector/update.py

import requests
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def update_vector(
    vector_id: str, 
    update_data: Dict[str, Any]
) -> Dict[str, Any]:
    """
    Update an existing vector in the database.
    
    Args:
        vector_id (str): The UUID of the vector to update
        update_data (Dict[str, Any]): Dictionary containing the fields to update
            Possible fields:
            - embedding (List[float]): The vector embedding array
            - content (str): The text content that was embedded
            - metadata (Dict): Additional metadata
            - namespace (str): Namespace for vector grouping
            
    Returns:
        Dict[str, Any]: The updated vector data
        
    Raises:
        Exception: If the vector is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the updated resource
    }
    
    # Send PATCH request to update the vector
    response = requests.patch(
        f"{POSTGREST_BASE_URL}/vector_store?vector_id=eq.{vector_id}",
        headers=headers,
        json=update_data
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Vector not found with ID: {vector_id}")
    else:
        error_message = f"Failed to update vector: {response.status_code} - {response.text}"
        raise Exception(error_message) 
# tools/database/vector/read.py

import requests
import json
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def get_vector(vector_id: str) -> Dict[str, Any]:
    """
    Retrieve a specific vector by its ID.
    
    Args:
        vector_id (str): The UUID of the vector to retrieve
        
    Returns:
        Dict[str, Any]: The vector data
        
    Raises:
        Exception: If the vector is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send GET request to retrieve the vector
    response = requests.get(
        f"{POSTGREST_BASE_URL}/vectors?vector_id=eq.{vector_id}",
        headers=headers
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Vector not found with ID: {vector_id}")
    else:
        error_message = f"Failed to retrieve vector: {response.status_code} - {response.text}"
        raise Exception(error_message)

def list_vectors(
    thread_id: Optional[str] = None,
    message_id: Optional[str] = None,
    namespace: Optional[str] = None,
    limit: int = 100,
    offset: int = 0
) -> List[Dict[str, Any]]:
    """
    List vectors with optional filtering.
    
    Args:
        thread_id (str, optional): Filter by thread ID. Defaults to None.
        message_id (str, optional): Filter by message ID. Defaults to None.
        namespace (str, optional): Filter by namespace. Defaults to None.
        limit (int, optional): Maximum number of vectors to return. Defaults to 100.
        offset (int, optional): Number of vectors to skip. Defaults to 0.
            
    Returns:
        List[Dict[str, Any]]: List of vector objects
        
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
    url = f"{POSTGREST_BASE_URL}/vectors"
    
    # Add query parameters
    params = {
        "limit": limit,
        "offset": offset,
        "order": "created_at.desc"
    }
    
    # Add filter for thread_id if provided
    if thread_id:
        params["thread_id"] = f"eq.{thread_id}"
    
    # Add filter for message_id if provided
    if message_id:
        params["message_id"] = f"eq.{message_id}"
    
    # Add filter for namespace if provided
    if namespace:
        params["namespace"] = f"eq.{namespace}"
    
    # Send GET request to retrieve vectors
    response = requests.get(url, headers=headers, params=params)
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to list vectors: {response.status_code} - {response.text}"
        raise Exception(error_message)

def search_vectors(
    query_embedding: List[float],
    namespace: str = "default",
    thread_id: Optional[str] = None,
    similarity_threshold: float = 0.7,
    limit: int = 10
) -> List[Dict[str, Any]]:
    """
    Search for similar vectors using cosine similarity.
    
    Note: This function directly uses a PostgreSQL function that performs
    the vector similarity search using pgvector.
    
    Args:
        query_embedding (List[float]): The vector embedding to search with
        namespace (str, optional): Filter by namespace. Defaults to "default".
        thread_id (Optional[str], optional): Filter by thread ID. Defaults to None.
        similarity_threshold (float, optional): Minimum similarity score (0-1). Defaults to 0.7.
        limit (int, optional): Maximum number of results. Defaults to 10.
        
    Returns:
        List[Dict[str, Any]]: List of vectors with similarity scores
        
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
    
    # Prepare the request parameters for the search_vector RPC function
    search_params = {
        "query_embedding": query_embedding,
        "namespace": namespace,
        "similarity_threshold": similarity_threshold,
        "match_count": limit,
        "thread_id": thread_id
    }
    
    # Send POST request to the RPC function for vector search
    response = requests.post(
        f"{POSTGREST_BASE_URL}/rpc/search_vectors",
        headers=headers,
        json=search_params
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to search vectors: {response.status_code} - {response.text}"
        raise Exception(error_message) 
# tools/database/vector/create.py

import requests
import json
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL
from tools.logger import logger

def create_vector(
    thread_id: str,
    message_id: Optional[str],
    embedding: List[float],
    content: str,
    metadata: Optional[Dict[str, Any]] = None,
    namespace: str = "default",
    embed_tool: Optional[Dict[str, Any]] = None
) -> Dict[str, Any]:
    """
    Create a new vector embedding in the database.
    
    Args:
        thread_id (str): The ID of the thread this vector belongs to (prefixed with 'thread-')
        message_id (Optional[str]): The ID of the message, if applicable (prefixed with 'message-')
        embedding (List[float]): The vector embedding array
        content (str): The original text content that was embedded
        metadata (Dict[str, Any], optional): Additional metadata. Defaults to {}.
        namespace (str, optional): Namespace for vector grouping. Defaults to "default".
        embed_tool (Dict[str, Any], optional): Information about the tool used for embedding. Defaults to {}.
        
    Returns:
        Dict[str, Any]: The created vector data including vector_id (prefixed with 'vector-')
        
    Raises:
        Exception: If the API request fails
    """
    logger.debug(f"Creating vector for thread {thread_id}, namespace {namespace}")
    
    # Prepare the request data
    if metadata is None:
        metadata = {}
    
    # Add message_id and namespace to metadata for searching
    metadata_with_extras = metadata.copy()
    if message_id:
        metadata_with_extras["message_id"] = message_id
    metadata_with_extras["namespace"] = namespace
    
    vector_data = {
        "thread_id": thread_id,
        "embedding": embedding,
        "content": content,
        "metadata": metadata_with_extras,
        "embed_tool": embed_tool or {}
    }
    
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the created resource
    }
    
    # Send POST request to create the vector
    try:
        logger.debug(f"Sending vector creation request to {POSTGREST_BASE_URL}/vector_store")
        response = requests.post(
            f"{POSTGREST_BASE_URL}/vector_store",  # Use correct table name: vector_store
            headers=headers,
            json=vector_data
        )
        
        # Check if the request was successful
        if response.status_code == 201:
            result = response.json()[0]  # PostgREST returns array with single item
            logger.info(f"Successfully created vector with ID: {result.get('vector_id', 'unknown')}")
            return result
        else:
            error_message = f"Failed to create vector: {response.status_code} - {response.text}"
            logger.error(error_message)
            raise Exception(error_message)
    except Exception as e:
        logger.error(f"Exception creating vector: {str(e)}")
        raise 
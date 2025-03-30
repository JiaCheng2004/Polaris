# tools/database/vector/read.py

import requests
import json
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL
from tools.logger import logger

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
    logger.debug(f"Getting vector with ID: {vector_id}")
    
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send GET request to retrieve the vector
    try:
        response = requests.get(
            f"{POSTGREST_BASE_URL}/vector_store?vector_id=eq.{vector_id}",
            headers=headers
        )
        
        # Check if the request was successful
        if response.status_code == 200:
            results = response.json()
            if results:
                logger.debug(f"Successfully retrieved vector: {vector_id}")
                return results[0]
            else:
                error_message = f"Vector not found with ID: {vector_id}"
                logger.warning(error_message)
                raise Exception(error_message)
        else:
            error_message = f"Failed to retrieve vector: {response.status_code} - {response.text}"
            logger.error(error_message)
            raise Exception(error_message)
    except Exception as e:
        logger.error(f"Exception getting vector {vector_id}: {str(e)}")
        raise

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
    logger.debug(f"Listing vectors for thread: {thread_id}, message: {message_id}, namespace: {namespace}")
    
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Build the base URL
    url = f"{POSTGREST_BASE_URL}/vector_store"
    
    # Add query parameters
    params = {}
    query_parts = []
    
    # Add filter for thread_id if provided
    if thread_id:
        query_parts.append(f"thread_id=eq.{thread_id}")
    
    # Add filter for message_id if provided (in metadata)
    if message_id:
        query_parts.append(f"metadata->>'message_id'=eq.{message_id}")
    
    # Add filter for namespace if provided (in metadata)
    if namespace:
        query_parts.append(f"metadata->>'namespace'=eq.{namespace}")
    
    # Construct the URL with filters
    if query_parts:
        url = f"{url}?{query_parts[0]}"
        for part in query_parts[1:]:
            url = f"{url}&{part}"
    
    # Add pagination and ordering
    params["limit"] = limit
    params["offset"] = offset
    params["order"] = "created_at.desc"
    
    # Send GET request to retrieve vectors
    try:
        logger.debug(f"Sending list vectors request to: {url}")
        response = requests.get(url, headers=headers, params=params)
        
        # Check if the request was successful
        if response.status_code == 200:
            results = response.json()
            logger.info(f"Found {len(results)} vectors")
            return results
        else:
            error_message = f"Failed to list vectors: {response.status_code} - {response.text}"
            logger.error(error_message)
            raise Exception(error_message)
    except Exception as e:
        logger.error(f"Exception listing vectors: {str(e)}")
        raise

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
    logger.debug(f"Searching vectors for thread: {thread_id}, namespace: {namespace}")
    
    # If no thread_id provided, return empty list
    if not thread_id:
        logger.warning("No thread_id provided for vector search")
        return []
        
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Send POST request to the RPC function for vector search
    try:
        logger.debug(f"Sending search vectors request to: {POSTGREST_BASE_URL}/rpc/search_vectors")
        
        # In PostgREST, function parameters need to match exactly in the body
        # Ensure parameter names match the PostgreSQL function definition
        response = requests.post(
            f"{POSTGREST_BASE_URL}/rpc/search_vectors",
            headers=headers,
            json={
                "query_embedding": query_embedding,
                "namespace": namespace,
                "thread_id_param": thread_id,
                "similarity_threshold": similarity_threshold,
                "match_count": limit
            }
        )
        
        # Check if the request was successful
        if response.status_code == 200:
            results = response.json()
            logger.info(f"Found {len(results)} similar vectors")
            return results
        else:
            error_message = f"Failed to search vectors: {response.status_code} - {response.text}"
            logger.error(error_message)
            
            # Look for specific parameter mismatch errors
            if response.status_code == 404 and "Could not find the function" in response.text:
                logger.error("PostgreSQL function parameter mismatch detected. Check that parameter names and order match the database function definition.")
                
            # We'll continue to the fallbacks
    except Exception as e:
        logger.error(f"Exception searching vectors: {str(e)}")

    # First fallback: Try using get_thread_vectors function
    try:
        logger.info(f"Trying get_thread_vectors fallback for thread: {thread_id}")
        
        # In PostgREST, function parameters need to match exactly in the body
        # Ensure parameter names match the PostgreSQL function definition
        response = requests.post(
            f"{POSTGREST_BASE_URL}/rpc/get_thread_vectors",
            headers=headers,
            json={
                "thread_id_param": thread_id,
                "namespace_param": namespace,
                "limit_param": limit * 3  # Get more vectors for better selection
            }
        )
        
        # Check if the request was successful
        if response.status_code == 200:
            vectors = response.json()
            if not vectors:
                logger.info("No vectors found using get_thread_vectors")
                return []
                
            # We need to compute similarity manually
            from numpy import dot
            from numpy.linalg import norm
            
            def cosine_similarity(a, b):
                return dot(a, b) / (norm(a) * norm(b))
            
            # Calculate similarity for each vector
            results_with_scores = []
            for vector in vectors:
                if "embedding" in vector and vector["embedding"]:
                    try:
                        similarity = cosine_similarity(query_embedding, vector["embedding"])
                        if similarity >= similarity_threshold:
                            # Add similarity score to the vector object
                            vector_copy = vector.copy()
                            vector_copy["similarity"] = float(similarity)
                            results_with_scores.append(vector_copy)
                    except Exception as calc_error:
                        logger.warning(f"Error calculating similarity: {str(calc_error)}")
            
            # Sort by similarity score (descending)
            results_with_scores.sort(key=lambda x: x.get("similarity", 0), reverse=True)
            
            # Limit results
            limited_results = results_with_scores[:limit]
            
            logger.info(f"Found {len(limited_results)} similar vectors using get_thread_vectors")
            return limited_results
        else:
            logger.error(f"Failed get_thread_vectors: {response.status_code} - {response.text}")
            
            # Look for specific parameter mismatch errors
            if response.status_code == 404 and "Could not find the function" in response.text:
                logger.error("PostgreSQL function parameter mismatch detected. Check that parameter names and order match the database function definition.")
    except Exception as gtv_error:
        logger.error(f"Exception in get_thread_vectors fallback: {str(gtv_error)}")
    
    # Second fallback: Try to get vectors directly using list_vectors
    try:
        logger.info(f"Trying list_vectors fallback for thread: {thread_id}")
        vectors = list_vectors(thread_id=thread_id, namespace=namespace, limit=100)
        
        if not vectors:
            logger.info("No vectors found for thread in list_vectors fallback")
            return []
            
        # We need to compute similarity manually
        from numpy import dot
        from numpy.linalg import norm
        
        def cosine_similarity(a, b):
            return dot(a, b) / (norm(a) * norm(b))
        
        # Calculate similarity for each vector
        results_with_scores = []
        for vector in vectors:
            if "embedding" in vector and vector["embedding"]:
                try:
                    similarity = cosine_similarity(query_embedding, vector["embedding"])
                    if similarity >= similarity_threshold:
                        # Add similarity score to the vector object
                        vector_copy = vector.copy()
                        vector_copy["similarity"] = float(similarity)
                        results_with_scores.append(vector_copy)
                except Exception as calc_error:
                    logger.warning(f"Error calculating similarity: {str(calc_error)}")
        
        # Sort by similarity score (descending)
        results_with_scores.sort(key=lambda x: x.get("similarity", 0), reverse=True)
        
        # Limit results
        limited_results = results_with_scores[:limit]
        
        logger.info(f"Found {len(limited_results)} similar vectors using list_vectors fallback")
        return limited_results
    except Exception as fallback_error:
        logger.error(f"All fallbacks failed: {str(fallback_error)}")
        # Return empty list rather than failing completely
        return [] 
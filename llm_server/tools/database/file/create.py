# tools/database/file/create.py

import requests
import json
import hashlib
import os
from typing import Dict, Any, Optional, Union
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def create_file(
    filename: str,
    file_type: str,
    size: int,
    content: str = "",
    author: Optional[Dict[str, Any]] = None,
    address: str = "",
    token_count: int = 0,
    metadata: Optional[Dict[str, Any]] = None,
    parse_tool: Optional[Dict[str, Any]] = None,
    content_hash: Optional[str] = None
) -> Dict[str, Any]:
    """
    Create a new file in the database.
    
    Args:
        filename (str): The name of the file
        file_type (str): MIME type of the file (e.g., "application/pdf", "image/png")
        size (int): File size in bytes
        content (str, optional): The file content as a string for text files, empty for binary files
        author (Optional[Dict[str, Any]], optional): JSON data describing who uploaded or generated it, or None
        address (str): Path to the physical file or "deleted" if removed
        token_count (int, optional): Number of tokens extracted. Defaults to 0.
        metadata (Dict[str, Any], optional): Extra metadata about the file. Defaults to {}.
        parse_tool (Dict[str, Any], optional): Information about the tool used for parsing. Defaults to {}.
        content_hash (str, optional): Pre-computed content hash. If not provided, will be generated.
        
    Returns:
        Dict[str, Any]: The created file data including file_id (prefixed with 'file-')
        
    Raises:
        Exception: If the API request fails
    """
    # Use provided content hash or generate one
    if not content_hash:
        # If content is available, use it for the hash
        if content:
            content_hash = hashlib.sha256(content.encode()).hexdigest()
        else:
            # For binary files without content string, use address and timestamp for uniqueness
            unique_str = f"{address}:{filename}:{size}:{os.urandom(8).hex()}"
            content_hash = hashlib.sha256(unique_str.encode()).hexdigest()
    
    # Use default empty JSON object for author if None
    default_author = {"type": "unknown", "id": "system"}
    
    # Prepare the request data
    file_data = {
        "filename": filename,
        "type": file_type,
        "size": size,
        "content": content or "",  # Ensure content is never None
        "token_count": token_count,
        "content_hash": content_hash,
        "author": author or default_author,
        "address": address,
        "metadata": metadata or {},
        "parse_tool": parse_tool or {}
    }
    
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the created resource
    }
    
    # Send POST request to create the file
    response = requests.post(
        f"{POSTGREST_BASE_URL}/files",
        headers=headers,
        json=file_data
    )
    
    # Check if the request was successful
    if response.status_code == 201:
        return response.json()[0]  # PostgREST returns array with single item
    else:
        error_message = f"Failed to create file: {response.status_code} - {response.text}"
        raise Exception(error_message)

def find_existing_file_by_hash(content_hash: str) -> Optional[Dict[str, Any]]:
    """
    Check if a file with the same content hash already exists.
    If found and marked as deleted, it can be updated with a new address.
    
    Args:
        content_hash (str): The SHA-256 hash of the file content
        
    Returns:
        Optional[Dict[str, Any]]: The existing file data if found, None otherwise
        
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
    
    # Send GET request to find the file by content hash
    response = requests.get(
        f"{POSTGREST_BASE_URL}/files?content_hash=eq.{content_hash}",
        headers=headers
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        return None
    else:
        error_message = f"Failed to search for file: {response.status_code} - {response.text}"
        raise Exception(error_message) 
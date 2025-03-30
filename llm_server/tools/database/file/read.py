# tools/database/file/read.py

import requests
import hashlib
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL
from tools.logger import logger

def get_file(file_id: str, include_content: bool = True) -> Dict[str, Any]:
    """
    Retrieve a specific file by its ID.
    
    Args:
        file_id (str): The UUID of the file to retrieve
        include_content (bool, optional): Whether to include the full content field. 
            Set to False for large files to reduce data transfer. Defaults to True.
        
    Returns:
        Dict[str, Any]: The file data
        
    Raises:
        Exception: If the file is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    
    # Build the URL with appropriate columns selection
    if include_content:
        url = f"{POSTGREST_BASE_URL}/files?file_id=eq.{file_id}"
    else:
        # Select all columns except content
        url = f"{POSTGREST_BASE_URL}/files?select=file_id,author,filename,type,size,token_count,metadata,content_hash,address,created_at,updated_at&file_id=eq.{file_id}"
    
    # Send GET request to retrieve the file
    response = requests.get(url, headers=headers)
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            # Verify content hash if content is included
            if include_content:
                file_data = results[0]
                content = file_data.get("content", "")
                stored_hash = file_data.get("content_hash", "")
                
                # Log hash verification attempt
                logger.debug(f"Verifying content hash for file ID: {file_id}")
                logger.debug(f"Stored hash: {stored_hash}")
                
                # Skip verification for binary files or empty content
                if not content:
                    logger.debug(f"Skipping hash verification for file {file_id}: empty content string (likely binary file)")
                    return file_data
                
                # Compute hash from content string
                computed_hash = hashlib.sha256(content.encode()).hexdigest()
                logger.debug(f"Computed hash: {computed_hash}")
                
                if stored_hash and computed_hash != stored_hash:
                    # Log detailed hash mismatch
                    logger.warning(f"File content hash mismatch for ID: {file_id}")
                    logger.warning(f"  - Stored hash:   {stored_hash}")
                    logger.warning(f"  - Computed hash: {computed_hash}")
                    logger.warning(f"  - Content length: {len(content)} characters")
                    
                    # Check if this is a binary file (content in DB doesn't match actual file)
                    file_type = file_data.get("type", "")
                    if "text/" not in file_type and "application/json" not in file_type:
                        logger.warning(f"  - This appears to be a binary file ({file_type}). Hash mismatch is expected.")
                    
                    # Continue despite hash mismatch
                    logger.info(f"Proceeding with file {file_id} despite hash mismatch")
            
            return results[0]
        else:
            raise Exception(f"File not found with ID: {file_id}")
    else:
        error_message = f"Failed to retrieve file: {response.status_code} - {response.text}"
        raise Exception(error_message)

def list_files(
    content_hash: Optional[str] = None,
    filename: Optional[str] = None,
    file_type: Optional[str] = None,
    limit: int = 100,
    offset: int = 0,
    include_content: bool = False
) -> List[Dict[str, Any]]:
    """
    List files with optional filtering.
    
    Args:
        content_hash (str, optional): Filter by content hash. Defaults to None.
        filename (str, optional): Filter by filename. Defaults to None.
        file_type (str, optional): Filter by file type. Defaults to None.
        limit (int, optional): Maximum number of files to return. Defaults to 100.
        offset (int, optional): Number of files to skip. Defaults to 0.
        include_content (bool, optional): Whether to include the full content field.
            Set to False to reduce data transfer. Defaults to False.
            
    Returns:
        List[Dict[str, Any]]: List of file objects
        
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
    
    # Determine which columns to select based on include_content
    columns = "*" if include_content else "file_id,author,filename,type,size,token_count,metadata,content_hash,address,created_at,updated_at"
    
    # Build the base URL with selected columns
    url = f"{POSTGREST_BASE_URL}/files?select={columns}"
    
    # Add query parameters
    params = {
        "limit": limit,
        "offset": offset,
        "order": "created_at.desc"
    }
    
    # Add filter conditions if provided
    if content_hash:
        params["content_hash"] = f"eq.{content_hash}"
    
    if filename:
        params["filename"] = f"eq.{filename}"
    
    if file_type:
        params["type"] = f"eq.{file_type}"
    
    # Send GET request to retrieve files
    response = requests.get(url, headers=headers, params=params)
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to list files: {response.status_code} - {response.text}"
        raise Exception(error_message)

def find_files_by_content_hash(content_hash: str, include_content: bool = False) -> List[Dict[str, Any]]:
    """
    Find files with a specific content hash.
    
    Args:
        content_hash (str): The content hash to search for
        include_content (bool, optional): Whether to include the full content field.
            Set to False to reduce data transfer. Defaults to False.
            
    Returns:
        List[Dict[str, Any]]: List of file objects with matching content hash
        
    Raises:
        Exception: If the API request fails
    """
    return list_files(content_hash=content_hash, include_content=include_content) 
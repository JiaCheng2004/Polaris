# tools/database/file/update.py

import requests
import hashlib
from typing import Dict, Any, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL
from tools.logger import logger

def update_file(
    file_id: str,
    address: Optional[str] = None,
    metadata: Optional[Dict[str, Any]] = None,
    token_count: Optional[int] = None,
    parse_tool: Optional[Dict[str, Any]] = None
) -> Dict[str, Any]:
    """
    Update a file in the database.
    
    Args:
        file_id (str): The ID of the file to update
        address (str, optional): New path to the file or "deleted" if removed. Defaults to None.
        metadata (Dict[str, Any], optional): Updated metadata. Defaults to None.
        token_count (int, optional): Updated token count. Defaults to None.
        parse_tool (Dict[str, Any], optional): Updated parse tool information. Defaults to None.
        
    Returns:
        Dict[str, Any]: The updated file data
        
    Raises:
        Exception: If the file is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the updated resource
    }
    
    # Prepare the update data with only provided fields
    update_data = {}
    
    if address is not None:
        update_data["address"] = address
        
    if metadata is not None:
        update_data["metadata"] = metadata
        
    if token_count is not None:
        update_data["token_count"] = token_count
        
    if parse_tool is not None:
        update_data["parse_tool"] = parse_tool
    
    # If no updates provided, return the current file data
    if not update_data:
        return get_file(file_id)
    
    # Send PATCH request to update the file
    response = requests.patch(
        f"{POSTGREST_BASE_URL}/files?file_id=eq.{file_id}",
        headers=headers,
        json=update_data
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"File not found with ID: {file_id}")
    else:
        error_message = f"Failed to update file: {response.status_code} - {response.text}"
        raise Exception(error_message)

def update_file_address(file_id: str, address: str) -> Dict[str, Any]:
    """
    Update a file's address.
    This is especially useful when a file that was marked as "deleted" is uploaded again.
    
    Args:
        file_id (str): The ID of the file to update
        address (str): New path to the file
        
    Returns:
        Dict[str, Any]: The updated file data
        
    Raises:
        Exception: If the file is not found or the API request fails
    """
    return update_file(file_id=file_id, address=address)

def mark_file_as_deleted(file_id: str) -> Dict[str, Any]:
    """
    Mark a file as deleted without removing it from the database.
    This sets the address field to "deleted".
    
    Args:
        file_id (str): The ID of the file to mark as deleted
        
    Returns:
        Dict[str, Any]: The updated file data
        
    Raises:
        Exception: If the file is not found or the API request fails
    """
    return update_file(file_id=file_id, address="deleted")

def update_file_by_content_hash(content_hash: str, address: str) -> Optional[Dict[str, Any]]:
    """
    Update the address of a file with a matching content hash.
    This is used when a file with the same content is uploaded again.
    
    Args:
        content_hash (str): The content hash to match
        address (str): New path to the file
        
    Returns:
        Optional[Dict[str, Any]]: The updated file data if found, None otherwise
        
    Raises:
        Exception: If the API request fails
    """
    from .read import find_files_by_content_hash
    
    logger.info(f"Updating file by content hash: {content_hash[:8]}...")
    
    # Find files with this content hash
    try:
        matching_files = find_files_by_content_hash(content_hash)
        if matching_files:
            logger.info(f"Found {len(matching_files)} matching files with content hash: {content_hash[:8]}...")
            
            # Log details about the first match
            first_match = matching_files[0]
            logger.debug(f"First matching file details:")
            logger.debug(f"  - File ID: {first_match.get('file_id', 'unknown')}")
            logger.debug(f"  - Filename: {first_match.get('filename', 'unknown')}")
            logger.debug(f"  - Current address: {first_match.get('address', 'unknown')}")
            logger.debug(f"  - Content hash: {first_match.get('content_hash', 'unknown')}")
            logger.debug(f"  - Size: {first_match.get('size', 0)} bytes")
            
            # Update the address of the first matching file
            file_id = first_match["file_id"]
            logger.info(f"Updating address for file ID {file_id} to {address}")
            
            updated_file = update_file(file_id=file_id, address=address)
            logger.info(f"Successfully updated file address for file ID: {file_id}")
            return updated_file
        else:
            logger.warning(f"No matching files found with content hash: {content_hash}")
            return None
    except Exception as e:
        logger.error(f"Error updating file by content hash {content_hash}: {str(e)}")
        # Re-raise the exception
        raise 
# tools/database/file/delete.py

import requests
from typing import Dict, Any, List
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def delete_file(file_id: str) -> Dict[str, Any]:
    """
    Delete a file from the database.
    Note: This completely removes the file record from the database.
    If you want to keep the record but mark it as deleted, use mark_file_as_deleted() instead.
    
    Args:
        file_id (str): The ID of the file to delete
        
    Returns:
        Dict[str, Any]: The deleted file data
        
    Raises:
        Exception: If the file is not found or the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the deleted resource
    }
    
    # Send DELETE request to remove the file
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/files?file_id=eq.{file_id}",
        headers=headers
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"File not found with ID: {file_id}")
    else:
        error_message = f"Failed to delete file: {response.status_code} - {response.text}"
        raise Exception(error_message)

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
    from .update import update_file
    return update_file(file_id=file_id, address="deleted")

def delete_files_by_content_hash(content_hash: str) -> List[Dict[str, Any]]:
    """
    Delete all files with a specific content hash.
    
    Args:
        content_hash (str): The content hash of files to delete
        
    Returns:
        List[Dict[str, Any]]: The deleted file data
        
    Raises:
        Exception: If the API request fails
    """
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the deleted resources
    }
    
    # Send DELETE request to remove files with matching content hash
    response = requests.delete(
        f"{POSTGREST_BASE_URL}/files?content_hash=eq.{content_hash}",
        headers=headers
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to delete files: {response.status_code} - {response.text}"
        raise Exception(error_message) 
# tools/database/attachment/update.py

import requests
import hashlib
from typing import Dict, Any, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def update_attachment(
    file_id: str, 
    update_data: Dict[str, Any]
) -> Dict[str, Any]:
    """
    Update an existing attachment in the database.
    
    Args:
        file_id (str): The UUID of the attachment to update
        update_data (Dict[str, Any]): Dictionary containing the fields to update
            Possible fields:
            - filename (str): The name of the file
            - metadata (Dict): Extra metadata about the file
            - purpose (str): The purpose of the attachment
            - token_count (int): Number of tokens extracted
            
            Note: Updating the content field requires special handling to 
            maintain content hash integrity. Use update_attachment_content instead.
            
    Returns:
        Dict[str, Any]: The updated attachment data
        
    Raises:
        Exception: If the attachment is not found or the API request fails
        ValueError: If attempting to update content directly
    """
    # Prevent direct content updates without hash recalculation
    if "content" in update_data:
        raise ValueError("Direct content updates not allowed. Use update_attachment_content instead.")
    
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the updated resource
    }
    
    # Send PATCH request to update the attachment
    response = requests.patch(
        f"{POSTGREST_BASE_URL}/attachments?file_id=eq.{file_id}",
        headers=headers,
        json=update_data
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Attachment not found with ID: {file_id}")
    else:
        error_message = f"Failed to update attachment: {response.status_code} - {response.text}"
        raise Exception(error_message)

def update_attachment_content(
    file_id: str,
    content: str,
    update_size: bool = True
) -> Dict[str, Any]:
    """
    Update the content of an existing attachment, recalculating the hash.
    
    Args:
        file_id (str): The UUID of the attachment to update
        content (str): The new file content
        update_size (bool, optional): Whether to update the size field based on content length.
            Defaults to True.
            
    Returns:
        Dict[str, Any]: The updated attachment data
        
    Raises:
        Exception: If the attachment is not found or the API request fails
    """
    # Generate content hash for integrity
    content_hash = hashlib.sha256(content.encode()).hexdigest()
    
    # Prepare the update data
    update_data = {
        "content": content,
        "content_hash": content_hash
    }
    
    # Update size if requested
    if update_size:
        update_data["size"] = len(content.encode('utf-8'))
    
    # Generate auth token for PostgREST
    token = generate_token()
    
    # Set up headers with auth token
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Prefer": "return=representation"  # Return the updated resource
    }
    
    # Send PATCH request to update the attachment
    response = requests.patch(
        f"{POSTGREST_BASE_URL}/attachments?file_id=eq.{file_id}",
        headers=headers,
        json=update_data
    )
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            return results[0]
        else:
            raise Exception(f"Attachment not found with ID: {file_id}")
    else:
        error_message = f"Failed to update attachment content: {response.status_code} - {response.text}"
        raise Exception(error_message) 
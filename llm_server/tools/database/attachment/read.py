# tools/database/attachment/read.py

import requests
import hashlib
from typing import Dict, Any, List, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def get_attachment(file_id: str, include_content: bool = True) -> Dict[str, Any]:
    """
    Retrieve a specific attachment by its ID.
    
    Args:
        file_id (str): The UUID of the attachment to retrieve
        include_content (bool, optional): Whether to include the full content field. 
            Set to False for large files to reduce data transfer. Defaults to True.
        
    Returns:
        Dict[str, Any]: The attachment data
        
    Raises:
        Exception: If the attachment is not found or the API request fails
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
        url = f"{POSTGREST_BASE_URL}/attachments?file_id=eq.{file_id}"
    else:
        # Select all columns except content
        url = f"{POSTGREST_BASE_URL}/attachments?select=file_id,message_id,author,filename,type,size,token_count,metadata,content_hash,purpose,created_at,updated_at&file_id=eq.{file_id}"
    
    # Send GET request to retrieve the attachment
    response = requests.get(url, headers=headers)
    
    # Check if the request was successful
    if response.status_code == 200:
        results = response.json()
        if results:
            # Verify content hash if content is included
            if include_content:
                attachment = results[0]
                content = attachment.get("content", "")
                stored_hash = attachment.get("content_hash", "")
                computed_hash = hashlib.sha256(content.encode()).hexdigest()
                
                if stored_hash and computed_hash != stored_hash:
                    raise Exception(f"Attachment content hash mismatch for ID: {file_id}")
            
            return results[0]
        else:
            raise Exception(f"Attachment not found with ID: {file_id}")
    else:
        error_message = f"Failed to retrieve attachment: {response.status_code} - {response.text}"
        raise Exception(error_message)

def list_attachments(
    message_id: Optional[str] = None,
    purpose: Optional[str] = None,
    limit: int = 100,
    offset: int = 0,
    include_content: bool = False
) -> List[Dict[str, Any]]:
    """
    List attachments with optional filtering.
    
    Args:
        message_id (str, optional): Filter by message ID. Defaults to None.
        purpose (str, optional): Filter by attachment purpose. Defaults to None.
        limit (int, optional): Maximum number of attachments to return. Defaults to 100.
        offset (int, optional): Number of attachments to skip. Defaults to 0.
        include_content (bool, optional): Whether to include the full content field.
            Set to False to reduce data transfer. Defaults to False.
            
    Returns:
        List[Dict[str, Any]]: List of attachment objects
        
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
    columns = "*" if include_content else "file_id,message_id,author,filename,type,size,token_count,metadata,content_hash,purpose,created_at,updated_at"
    
    # Build the base URL with selected columns
    url = f"{POSTGREST_BASE_URL}/attachments?select={columns}"
    
    # Add query parameters
    params = {
        "limit": limit,
        "offset": offset,
        "order": "created_at.desc"
    }
    
    # Add filter conditions if provided
    if message_id:
        params["message_id"] = f"eq.{message_id}"
    
    if purpose:
        params["purpose"] = f"eq.{purpose}"
    
    # Send GET request to retrieve attachments
    response = requests.get(url, headers=headers, params=params)
    
    # Check if the request was successful
    if response.status_code == 200:
        return response.json()
    else:
        error_message = f"Failed to list attachments: {response.status_code} - {response.text}"
        raise Exception(error_message)

def get_message_attachments(message_id: str, include_content: bool = False) -> List[Dict[str, Any]]:
    """
    Get all attachments for a specific message.
    
    Args:
        message_id (str): The UUID of the message
        include_content (bool, optional): Whether to include the full content field.
            Set to False to reduce data transfer. Defaults to False.
            
    Returns:
        List[Dict[str, Any]]: List of attachment objects
        
    Raises:
        Exception: If the API request fails
    """
    return list_attachments(message_id=message_id, include_content=include_content) 
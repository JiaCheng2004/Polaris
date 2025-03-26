# tools/database/attachment/create.py

import requests
import json
import hashlib
from typing import Dict, Any, Optional
from ..auth.token import generate_token
from tools.config.load import POSTGREST_BASE_URL

def create_attachment(
    message_id: str,
    filename: str,
    file_type: str,
    size: int,
    content: str,
    author: Dict[str, Any],
    purpose: str,
    token_count: int = 0,
    metadata: Optional[Dict[str, Any]] = None,
    parse_tool: Optional[Dict[str, Any]] = None
) -> Dict[str, Any]:
    """
    Create a new attachment in the database.
    
    Args:
        message_id (str): The ID of the message this attachment belongs to (prefixed with 'message-')
        filename (str): The name of the file
        file_type (str): MIME type of the file (e.g., "application/pdf", "image/png")
        size (int): File size in bytes
        content (str): The file content as a string
        author (Dict[str, Any]): JSON data describing who uploaded or generated it
        purpose (str): The purpose of the attachment (e.g., "reference", "embedded-image")
        token_count (int, optional): Number of tokens extracted. Defaults to 0.
        metadata (Dict[str, Any], optional): Extra metadata about the file. Defaults to {}.
        parse_tool (Dict[str, Any], optional): Information about the tool used for parsing. Defaults to {}.
        
    Returns:
        Dict[str, Any]: The created attachment data including file_id (prefixed with 'attachment-')
        
    Raises:
        Exception: If the API request fails
    """
    # Generate content hash for integrity
    content_hash = hashlib.sha256(content.encode()).hexdigest()
    
    # Prepare the request data
    attachment_data = {
        "message_id": message_id,
        "filename": filename,
        "type": file_type,
        "size": size,
        "content": content,
        "token_count": token_count,
        "content_hash": content_hash,
        "author": author,
        "purpose": purpose,
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
    
    # Send POST request to create the attachment
    response = requests.post(
        f"{POSTGREST_BASE_URL}/attachments",
        headers=headers,
        json=attachment_data
    )
    
    # Check if the request was successful
    if response.status_code == 201:
        return response.json()[0]  # PostgREST returns array with single item
    else:
        error_message = f"Failed to create attachment: {response.status_code} - {response.text}"
        raise Exception(error_message)
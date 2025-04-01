# tools/docs/attachment_utils.py
"""
Utilities for processing file attachments, parsing content, and vectorizing for retrieval.
These utilities are model-agnostic and can be used with any LLM integration.
"""

import os
from typing import List, Dict, Any, Optional
import traceback

from tools.database.file import get_file
from tools.database.vector.create import create_vector
from tools.embed.text import embed_text
from tools.parse.parser import Parse
from tools.docs.chunking import chunk_text
from tools.logger import logger

def process_attachments_for_vectorization(
    thread_id: str,
    file_ids: List[str],
    chunk_size: int,
    chunk_overlap: int,
    namespace: str = "files",
    embedding_model: Optional[Dict[str, Any]] = None
) -> None:
    """
    Process file attachments by parsing to text, chunking, and embedding.
    
    Args:
        thread_id: The thread ID for vector storage
        file_ids: List of file IDs to process
        chunk_size: Size of text chunks for embedding
        chunk_overlap: Overlap between chunks
        namespace: Vector namespace to use for storage
        embedding_model: Optional info about which embedding model to use
            Example: {"type": "openai", "model": "text-embedding-ada-002"}
    """
    parser = Parse()
    
    if not file_ids:
        logger.info("No file IDs provided for vectorization")
        return
    
    logger.info(f"Processing {len(file_ids)} files for vectorization")
    
    # Set default embedding model info if not provided
    if embedding_model is None:
        embedding_model = {"type": "embed", "model": "default"}
    
    # Get the configured upload directory from environment or use default paths
    # Use server configuration or fallback to default paths
    UPLOAD_DIRECTORIES = [
        "/app/uploads",           # Default Docker container path
        "/tmp/uploads",           # Temporary storage path
        "/var/tmp/uploads",       # Alternative temporary storage
        "/usr/src/app/uploads",   # Common Docker app path
        os.path.expanduser("~/uploads"),  # User home directory
        "./uploads"               # Current working directory
    ]
    
    logger.info(f"Searching for files in upload directories: {', '.join(UPLOAD_DIRECTORIES)}")
    
    for file_id in file_ids:
        try:
            # Get file details
            logger.debug(f"Getting details for file {file_id}")
            file_details = get_file(file_id)
            if not file_details:
                logger.warning(f"File not found: {file_id}")
                continue
            
            # Extract file details for logging
            filename = file_details.get("filename", "unknown")
            file_type = file_details.get("type", "unknown")
            address = file_details.get("address", "unknown")
            logger.info(f"Processing file: {filename} ({file_type}), ID: {file_id}, address: {address}")
            
            # Check if content is already in the database
            parsed_text = ""
            
            # Determine if we have content in the database
            has_content = "content" in file_details and file_details["content"]
            logger.debug(f"File has content in database: {has_content}")
            
            # Try to determine file path using address
            file_path = ""
            file_paths_checked = []
            
            if address and address != "deleted":
                # Start with direct path check (in case address is a full path)
                if os.path.exists(address) and os.path.isfile(address):
                    file_path = address
                    logger.debug(f"Found file at direct path: {file_path}")
                else:
                    # Try all possible upload directories
                    for upload_dir in UPLOAD_DIRECTORIES:
                        potential_path = os.path.join(upload_dir, address)
                        file_paths_checked.append(potential_path)
                        
                        logger.debug(f"Checking path: {potential_path}")
                        if os.path.exists(potential_path) and os.path.isfile(potential_path):
                            file_path = potential_path
                            logger.debug(f"Found file at path: {file_path}")
                            break
                
                if not file_path:
                    logger.error(f"Could not find file at any location. Address: {address}")
                    logger.error(f"Paths checked: {', '.join(file_paths_checked)}")
            else:
                logger.warning(f"File has no valid address or is marked as deleted: {address}")
            
            # Try multiple approaches to get file content
            if has_content:
                logger.info(f"Using existing content from database for file {file_id}")
                parsed_text = file_details["content"]
                logger.debug(f"Content length from database: {len(parsed_text)} characters")
            elif file_path and os.path.exists(file_path):
                # Parse file to text
                logger.debug(f"Parsing file at path: {file_path}")
                try:
                    parse_result = parser.parse(file_path)
                    
                    if parse_result["status"] == 200:
                        parsed_text = parse_result["content"]
                        logger.debug(f"Successfully parsed file, content length: {len(parsed_text)} characters")
                    else:
                        logger.warning(f"Parse error for file {file_id}: {parse_result.get('message', 'Unknown error')}")
                except Exception as parse_e:
                    logger.error(f"Error parsing file {file_id}: {str(parse_e)}")
            else:
                # Try to retrieve the file with docker-friendly path fix
                try:
                    # In some container setups, files might be accessible via different paths
                    # For example, host path /Users/user/uploads might be mounted as /app/uploads in container
                    # Try converting absolute host paths to container paths
                    if address and address.startswith("/Users/"):
                        # Try mapping /Users/* to Docker container paths
                        for prefix in ["/app", "/usr/src/app", "/tmp"]:
                            container_path = address.replace("/Users", prefix, 1)
                            logger.debug(f"Trying container path mapping: {container_path}")
                            
                            if os.path.exists(container_path) and os.path.isfile(container_path):
                                logger.info(f"Found file using container path mapping: {container_path}")
                                
                                # Parse file to text
                                parse_result = parser.parse(container_path)
                                
                                if parse_result["status"] == 200:
                                    parsed_text = parse_result["content"]
                                    logger.debug(f"Successfully parsed file, content length: {len(parsed_text)} characters")
                                    break
                except Exception as map_e:
                    logger.error(f"Error trying path mapping: {str(map_e)}")
                
                # If we still don't have content
                if not parsed_text:
                    # Both content and file path failed
                    logger.error(f"No content or valid file path for file {file_id}. Details:")
                    logger.error(f"  - Address: {address}")
                    logger.error(f"  - Has DB content: {has_content}")
                    logger.error(f"  - File path resolved: {file_path}")
                    logger.error(f"  - File exists check: {file_path and os.path.exists(file_path)}")
                    continue
            
            if not parsed_text:
                logger.warning(f"No content extracted from file {file_id}")
                continue
            
            # Log content length for debugging
            logger.debug(f"File content length: {len(parsed_text)} characters")
            
            # Split text into chunks
            logger.debug(f"Chunking text with size={chunk_size}, overlap={chunk_overlap}")
            text_chunks = chunk_text(
                text=parsed_text,
                chunk_size=chunk_size,
                chunk_overlap=chunk_overlap
            )
            
            logger.info(f"Created {len(text_chunks)} chunks from file {filename}")
            
            # Embed each chunk and store in vector database
            chunks_embedded = 0
            for i, chunk in enumerate(text_chunks):
                try:
                    logger.debug(f"Embedding chunk {i+1}/{len(text_chunks)}")
                    
                    # Generate embedding
                    embedding_list = embed_text(chunk)
                    if not embedding_list:
                        logger.warning(f"Failed to create embedding for chunk {i+1} from file {file_id}")
                        continue
                    
                    # Log type of embedding returned
                    logger.debug(f"Embedding type: {type(embedding_list).__name__}")
                    
                    # Verify embedding values are all numeric and finite
                    import math
                    valid_embedding = True
                    for j, val in enumerate(embedding_list[:10]):  # Check first few values
                        if not isinstance(val, float) or math.isnan(val) or math.isinf(val):
                            logger.error(f"Invalid embedding value at index {j}: {val}")
                            valid_embedding = False
                            break
                    
                    if not valid_embedding:
                        logger.warning(f"Embedding contains invalid values for chunk {i+1}, skipping")
                        continue
                    
                    # Store chunk and its embedding
                    metadata = {
                        "source": "file",
                        "file_id": file_id,
                        "file_name": file_details.get("filename", ""),
                        "file_type": file_details.get("type", ""),
                        "chunk_index": i
                    }
                    
                    try:
                        create_vector(
                            thread_id=thread_id,
                            message_id=None,
                            embedding=embedding_list,
                            content=chunk,
                            metadata=metadata,
                            namespace=namespace,
                            embed_tool=embedding_model
                        )
                        chunks_embedded += 1
                        logger.debug(f"Successfully stored vector for chunk {i+1}/{len(text_chunks)}")
                    except Exception as ve:
                        logger.error(f"Error storing vector in database for chunk {i+1}: {str(ve)}")
                    
                except Exception as chunk_e:
                    logger.error(f"Error processing chunk {i+1} from file {file_id}: {str(chunk_e)}")
                    logger.error(f"Chunk processing error traceback: {traceback.format_exc()}")
                    continue
                
            logger.info(f"Successfully embedded {chunks_embedded}/{len(text_chunks)} chunks from file {filename}")
        except Exception as e:
            # Special handling for common file-related errors
            error_message = str(e).lower()
            if "hash mismatch" in error_message or "content hash" in error_message:
                logger.error(f"File content hash mismatch for ID: {file_id}. This may indicate the file was modified after being added to the database.")
            elif "permission" in error_message:
                logger.error(f"Permission denied accessing file for ID: {file_id}")
            else:
                logger.error(f"Error processing file {file_id} for vectorization: {str(e)}")
                # Log more detail about the exception
                logger.error(f"Exception details: {traceback.format_exc()}") 
# tools/docs/message_utils.py
"""
Message processing utilities for handling conversation messages.
These utilities are model-agnostic and can be used with any LLM integration.
"""

from typing import Dict, List, Any, Optional
import traceback

from tools.database.message.create import create_message
from tools.database.vector.create import create_vector
from tools.embed.text import embed_text
from tools.logger import logger

def get_most_recent_user_query(messages: List[Dict[str, Any]]) -> Optional[Dict[str, Any]]:
    """
    Find the most recent message with role 'user'.
    
    Args:
        messages: List of message objects
        
    Returns:
        Optional[Dict[str, Any]]: The most recent user message, or None if not found
    """
    # Reverse the list to find the most recent user message
    for message in reversed(messages):
        if message.get("role") == "user":
            return message
    return None

def store_assistant_response(
    thread_id: str, 
    content: str, 
    user_author: Dict[str, Any],
    vectorize: bool = True,
    namespace: str = "messages",
    embedding_model: Optional[Dict[str, Any]] = None
) -> Dict[str, Any]:
    """
    Store the assistant's response and process for vector storage.
    
    Args:
        thread_id: The thread ID
        content: The response content
        user_author: The user author information (for context)
        vectorize: Whether to vectorize the response for retrieval
        namespace: Vector namespace to use for storage
        embedding_model: Optional info about which embedding model to use
            Example: {"type": "openai", "model": "text-embedding-ada-002"}
        
    Returns:
        Dict[str, Any]: The created message data
    """
    logger.info(f"Storing assistant response for thread {thread_id}")
    
    # Set default embedding model info if not provided
    if embedding_model is None:
        embedding_model = {"type": "embed", "model": "default"}
    
    # Validate content
    if not content:
        logger.warning("Empty content provided for assistant response")
        content = "I don't have a response at this time."
    
    # Create structured content
    structured_content = {"type": "text", "text": content}
    
    # Set assistant author
    assistant_author = {"id": "assistant", "name": "AI Assistant"}
    
    try:
        # Create the message
        logger.debug("Creating assistant message in database")
        created_message = create_message(
            thread_id=thread_id,
            role="assistant",
            content=structured_content,
            author=assistant_author
        )
        
        logger.info(f"Created assistant message with ID: {created_message['message_id']}")
        
        # Process response for vector storage only if significant content and vectorization is enabled
        if vectorize and len(content) > 10:  # Skip vectorization for very short responses
            try:
                # Generate embedding for the response
                logger.debug("Generating embedding for assistant response")
                embedding_list = embed_text(content)
                
                if embedding_list:
                    # Log embedding info
                    logger.debug(f"Assistant response embedding type: {type(embedding_list).__name__}, length: {len(embedding_list)}")
                    
                    # Store in vector database
                    logger.debug("Storing assistant response vector in database")
                    create_vector(
                        thread_id=thread_id,
                        message_id=created_message["message_id"],
                        embedding=embedding_list,
                        content=content,
                        metadata={"role": "assistant"},
                        namespace=namespace,
                        embed_tool=embedding_model
                    )
                    logger.info("Successfully stored assistant response vector")
                else:
                    logger.warning("Failed to generate embedding for assistant response")
            except Exception as vector_e:
                # Handle embedding/vector storage errors but still return the created message
                logger.error(f"Error storing vector for assistant response: {str(vector_e)}")
                logger.error(f"Content that failed to embed: {content[:100]}...")
                logger.error(f"Vector error traceback: {traceback.format_exc()}")
        else:
            if not vectorize:
                logger.info("Vectorization disabled for assistant response")
            else:
                logger.info("Skipping vectorization for short assistant response")
        
        return created_message
    except Exception as e:
        logger.error(f"Error storing assistant response: {str(e)}")
        logger.error(f"Error traceback: {traceback.format_exc()}")
        raise

def process_incoming_messages(
    thread_id: str,
    messages: List[Dict[str, Any]],
    author: Dict[str, Any],
    should_vectorize_attachments: bool = True,
    chunk_size: Optional[int] = None,
    chunk_overlap: Optional[int] = None,
    embedding_model: Optional[Dict[str, Any]] = None
) -> List[Dict[str, Any]]:
    """
    Process and store incoming messages with their attachments.
    
    Args:
        thread_id: The thread ID
        messages: List of message objects
        author: Author information
        should_vectorize_attachments: Whether to vectorize attachments
        chunk_size: Size of text chunks for embedding (uses config default if None)
        chunk_overlap: Overlap between chunks (uses config default if None)
        embedding_model: Optional info about which embedding model to use
            Example: {"type": "openai", "model": "text-embedding-ada-002"}
        
    Returns:
        List[Dict[str, Any]]: The created messages
    """
    logger.info(f"Processing {len(messages)} incoming messages for thread {thread_id}")
    
    # Import dependencies
    from tools.database.file import get_file
    from tools.docs.attachment_utils import process_attachments_for_vectorization
    from tools.config.load import DEFAULT_CHUNK_SIZE, DEFAULT_CHUNK_OVERLAP
    
    # Set default values from config if not provided
    if chunk_size is None:
        chunk_size = DEFAULT_CHUNK_SIZE
    if chunk_overlap is None:
        chunk_overlap = DEFAULT_CHUNK_OVERLAP
    if embedding_model is None:
        embedding_model = {"type": "embed", "model": "default"}
    
    created_messages = []
    
    for message_data in messages:
        # Extract message information
        role = message_data.get("role")
        content_text = message_data.get("content", "")
        file_ids = message_data.get("attachments", [])
        
        # Validate file IDs before proceeding
        if file_ids:
            valid_file_ids = []
            for file_id in file_ids:
                try:
                    file_data = get_file(file_id)
                    if file_data:
                        valid_file_ids.append(file_id)
                        logger.info(f"Successfully validated file ID: {file_id}")
                    else:
                        logger.warning(f"File not found for ID: {file_id}")
                except Exception as e:
                    error_message = str(e).lower()
                    if "hash mismatch" in error_message or "content hash" in error_message:
                        # Still consider the file valid if it's a hash mismatch
                        # This allows processing even if the file was updated
                        valid_file_ids.append(file_id)
                        logger.warning(f"File {file_id} has hash mismatch but will still be processed")
                    else:
                        logger.warning(f"Error validating file ID {file_id}: {str(e)}")
            
            # Use only the valid file IDs
            file_ids = valid_file_ids
            logger.info(f"Using {len(file_ids)} validated file IDs")
        
        # Structure content
        content = {"type": "text", "text": content_text}
        
        # Set author based on role
        if role == "user":
            msg_author = author
        elif role == "system":
            msg_author = {"id": "system", "name": "System"}
        else:  # assistant
            msg_author = {"id": "assistant", "name": "AI Assistant"}
        
        # Create the message
        try:
            created_message = create_message(
                thread_id=thread_id,
                role=role,
                content=content,
                author=msg_author,
                file_ids=file_ids
            )
            created_messages.append(created_message)
            logger.info(f"Created message with ID: {created_message['message_id']}")
            
            # Process attachments for vectorization if enabled
            if should_vectorize_attachments and file_ids:
                logger.info(f"Processing {len(file_ids)} attachments for vectorization")
                
                process_attachments_for_vectorization(
                    thread_id=thread_id,
                    file_ids=file_ids,
                    chunk_size=chunk_size,
                    chunk_overlap=chunk_overlap,
                    namespace="files",
                    embedding_model=embedding_model
                )
        except Exception as e:
            logger.error(f"Error creating message: {str(e)}")
    
    return created_messages 
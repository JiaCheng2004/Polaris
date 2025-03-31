# models/deepseek/deepseek_reasoner.py

import json
import uuid
import requests
from typing import Any, Dict, List, Optional, Tuple, Union
import os

# Database imports
from tools.database.thread.create import create_thread
from tools.database.thread.read import get_thread
from tools.database.message.create import create_message
from tools.database.vector.create import create_vector
from tools.database.vector.read import search_vectors
from tools.database.file import create_file, get_file

# Tool imports
from tools.embed.text import embed_text
from tools.parse.parser import Parse
from tools.llm.summarizer import summarize_context
from tools.llm.top_k_selector import get_optimal_top_k
from tools.config.load import DEFAULT_CHUNK_SIZE, DEFAULT_CHUNK_OVERLAP, DEEPSEEK_API_KEY
from tools.logger import logger

# Configure DeepSeek API endpoint
DEEPSEEK_API_URL = "https://api.deepseek.com/v1/chat/completions"

# Remove global client initialization to avoid potential API issues when module is loaded
# Instead create the client only when needed in the response generation function

def safely_convert_embedding_to_list(embedding: Any) -> Optional[List[float]]:
    """
    Safely convert various embedding types to a standard Python list.
    Handles ContentEmbedding and other non-standard embedding return types.
    
    Args:
        embedding: The embedding to convert
        
    Returns:
        Optional[List[float]]: The embedding as a list, or None if conversion failed
    """
    if embedding is None:
        return None
        
    # If it's already a list, return it directly
    if isinstance(embedding, list):
        # Verify all elements are floats
        try:
            return [float(x) for x in embedding]
        except (TypeError, ValueError) as e:
            logger.error(f"List conversion error - Failed to convert embedding values to float: {str(e)}")
            return None
    
    # Log the embedding type for debugging
    logger.debug(f"Converting embedding of type: {type(embedding).__name__}, repr: {repr(embedding)[:100]}...")
        
    # Handle Google Gemini's ContentEmbedding type specifically
    if hasattr(embedding, 'values') and callable(getattr(embedding, 'values', None)):
        try:
            # If values is a method that returns an iterable
            logger.debug("Using .values() method")
            values = embedding.values()
            return [float(x) for x in values]
        except Exception as e:
            logger.error(f"Using .values() method failed: {str(e)}")
    
    # Handle the specific ContentEmbedding case (where values is a property, not a method)
    if hasattr(embedding, 'values') and not callable(getattr(embedding, 'values', None)):
        try:
            logger.debug("Using .values property")
            # Convert to list and ensure all elements are floats
            values_list = [float(x) for x in embedding.values]
            logger.debug(f"Successfully converted embedding using .values property, length: {len(values_list)}")
            return values_list
        except Exception as e:
            logger.error(f"Using .values property failed: {str(e)}")
    
    # Try accessing embeddings attribute (Gemini embed_content response)
    if hasattr(embedding, 'embeddings'):
        try:
            logger.debug("Using .embeddings attribute")
            return [float(x) for x in embedding.embeddings]
        except Exception as e:
            logger.error(f"Using .embeddings attribute failed: {str(e)}")
            
    # Try different conversion methods
    try:
        # Try direct list conversion
        list_values = list(embedding)
        float_values = [float(x) for x in list_values]
        logger.debug(f"Direct list conversion successful, length: {len(float_values)}")
        return float_values
    except (TypeError, ValueError) as e:
        logger.error(f"Direct list conversion failed: {str(e)}")
        
    try:
        # Try using the embedding's to_list method if available
        if hasattr(embedding, 'to_list'):
            logger.debug("Using to_list() method")
            return [float(x) for x in embedding.to_list()]
    except Exception as e:
        logger.error(f"to_list() method failed: {str(e)}")
        
    try:
        # Try converting via __iter__ if available (for iterator-like objects)
        if hasattr(embedding, '__iter__'):
            logger.debug("Using __iter__ method")
            return [float(x) for x in embedding]
    except Exception as e:
        logger.error(f"Iteration conversion failed: {str(e)}")
        
    try:
        # Try getting embedding as a dictionary and converting values
        if hasattr(embedding, '__dict__'):
            logger.debug("Trying to convert from __dict__ attribute")
            embedding_dict = embedding.__dict__
            if isinstance(embedding_dict, dict) and any(isinstance(v, (int, float)) for v in embedding_dict.values()):
                return [float(v) for v in embedding_dict.values() if isinstance(v, (int, float))]
    except Exception as e:
        logger.error(f"Dict conversion failed: {str(e)}")
        
    # If none of the conversion methods worked
    logger.error(f"Failed to convert embedding of type {type(embedding).__name__} to list")
    return None

def create_deepseek_reasoner_completion(payload: Dict[str, Any], files: Optional[List[Any]] = None) -> Dict[str, Any]:
    """
    Main function to handle chat completions with DeepSeek Reasoner.
    
    Implements:
    1) Thread management (create new thread or use existing)
    2) Process files and store in vector database
    3) Retrieval of relevant documents for context
    4) Context management to fit into model's token limit
    5) LLM response generation
    
    Args:
        payload: The parsed JSON payload containing thread info and messages
        files: List of uploaded files (if any)
        
    Returns:
        Dict containing model response and metadata
    """
    logger.info("Starting DeepSeek Reasoner completion")
    
    try:
        # Extract fields from payload
        provider = payload.get("provider", "deepseek")
        model_name = payload.get("model", "deepseek-reasoner")
        messages = payload.get("messages", [])
        thread_id = payload.get("thread_id")
        purpose = payload.get("purpose", "chat")
        author = payload.get("author", {"type": "user", "user-id": "anonymous", "name": "User"})
        
        # Validate payload
        if not messages:
            logger.warning("No messages provided in payload")
            return {"error": "No messages provided in request payload"}
        
        logger.debug(f"Payload contains {len(messages)} messages")
        
        # Handle thread management
        try:
            thread_id = handle_thread_management(thread_id, model_name, provider, purpose, author)
            logger.info(f"Using thread ID: {thread_id}")
        except Exception as thread_e:
            logger.error(f"Thread management error: {str(thread_e)}")
            return {"error": f"Error in thread management: {str(thread_e)}"}
        
        # Store incoming messages and process attachments
        try:
            processed_messages = process_incoming_messages(thread_id, messages, author)
            logger.info(f"Processed {len(processed_messages)} messages")
        except Exception as msg_e:
            logger.error(f"Error processing messages: {str(msg_e)}")
            return {"error": f"Error processing messages: {str(msg_e)}"}
        
        # Find the most recent user query
        query_message = get_most_recent_user_query(processed_messages)
        if not query_message:
            logger.error("No user query found in messages")
            return {"error": "No user query found in messages"}
        
        logger.info(f"Found user query in message ID: {query_message.get('message_id', 'unknown')}")
        
        # Prepare context for LLM
        try:
            query_text, query_context_text, local_context_text = prepare_context_for_llm(thread_id, query_message)
            logger.debug(f"Prepared context - Query: {len(query_text)} chars, Query Context: {len(query_context_text)} chars, Local Context: {len(local_context_text)} chars")
        except Exception as context_e:
            logger.error(f"Error preparing context: {str(context_e)}")
            query_text = query_message.get("content", {}).get("text", "")
            query_context_text = ""
            local_context_text = ""
            logger.info("Using fallback context (query text only)")
        
        # Generate response from LLM
        try:
            response = generate_llm_response(query_text, query_context_text, local_context_text, model_name)
            logger.debug(f"Generated response: {len(response)} chars")
        except Exception as llm_e:
            logger.error(f"Error generating LLM response: {str(llm_e)}")
            response = f"I apologize, but I encountered an error processing your request: {str(llm_e)}"
        
        # Store assistant's response
        try:
            response_message = store_assistant_response(thread_id, response, author)
            logger.info(f"Stored assistant response as message ID: {response_message.get('message_id', 'unknown')}")
        except Exception as store_e:
            logger.error(f"Error storing assistant response: {str(store_e)}")
            # Create a basic response object with minimal info if we couldn't store in DB
            response_message = {
                "message_id": f"temp-{uuid.uuid4()}",
                "thread_id": thread_id,
                "tokens_spent": 0,
                "cost": 0.0
            }
        
        # Return the final response
        return {
            "thread_id": thread_id,
            "message_id": response_message.get("message_id", f"temp-{uuid.uuid4()}"),
            "content": response,
            "tokens_spent": response_message.get("tokens_spent", 0),
            "cost": response_message.get("cost", 0.0)
        }
    
    except Exception as e:
        # Catch-all for any unexpected errors
        logger.error(f"Unexpected error in deepseek_reasoner_completion: {str(e)}")
        import traceback
        logger.error(f"Traceback: {traceback.format_exc()}")
        return {
            "error": "An unexpected error occurred processing your request",
            "content": "I apologize, but I encountered an unexpected error processing your request. Please try again later."
        }

def handle_thread_management(
    thread_id: Optional[str],
    model_name: str,
    provider: str,
    purpose: str,
    author: Dict[str, Any]
) -> str:
    """
    Handle thread creation or verification.
    
    Args:
        thread_id: Optional thread ID for continuing a conversation
        model_name: The model being used
        provider: The provider of the model
        purpose: The purpose of the thread
        author: Information about the author
        
    Returns:
        str: The thread ID to use
    """
    logger.info(f"Handling thread management. Thread ID provided: {thread_id}")
    
    # Check if thread exists
    if thread_id:
        try:
            existing_thread = get_thread(thread_id)
            logger.info(f"Found existing thread: {thread_id}")
            return thread_id
        except Exception as e:
            logger.error(f"Error finding thread {thread_id}: {str(e)}")
            logger.info("Creating new thread since provided thread_id was not found")
    
    # Create new thread
    try:
        thread_data = create_thread(
            model=model_name,
            provider=provider,
            purpose=purpose,
            author=author
        )
        new_thread_id = thread_data["thread_id"]
        logger.info(f"Created new thread: {new_thread_id}")
        return new_thread_id
    except Exception as e:
        logger.error(f"Error creating thread: {str(e)}")
        raise

def process_incoming_messages(
    thread_id: str,
    messages: List[Dict[str, Any]],
    author: Dict[str, Any],
    should_vectorize_attachments: bool = True
) -> List[Dict[str, Any]]:
    """
    Process and store incoming messages with their attachments.
    
    Args:
        thread_id: The thread ID
        messages: List of message objects
        author: Author information
        should_vectorize_attachments: Whether to vectorize attachments
        
    Returns:
        List[Dict[str, Any]]: The created messages
    """
    logger.info(f"Processing {len(messages)} incoming messages for thread {thread_id}")
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
                    chunk_size=DEFAULT_CHUNK_SIZE,
                    chunk_overlap=DEFAULT_CHUNK_OVERLAP
                )
        except Exception as e:
            logger.error(f"Error creating message: {str(e)}")
    
    return created_messages

def process_attachments_for_vectorization(
    thread_id: str,
    file_ids: List[str],
    chunk_size: int,
    chunk_overlap: int
) -> None:
    """
    Process file attachments by parsing to text, chunking, and embedding.
    
    Args:
        thread_id: The thread ID for vector storage
        file_ids: List of file IDs to process
        chunk_size: Size of text chunks for embedding
        chunk_overlap: Overlap between chunks
    """
    parser = Parse()
    
    if not file_ids:
        logger.info("No file IDs provided for vectorization")
        return
    
    logger.info(f"Processing {len(file_ids)} files for vectorization")
    
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
                            namespace="files",
                            embed_tool={"type": "embed", "model": "gemini-embedding-exp-03-07"}
                        )
                        chunks_embedded += 1
                        logger.debug(f"Successfully stored vector for chunk {i+1}/{len(text_chunks)}")
                    except Exception as ve:
                        logger.error(f"Error storing vector in database for chunk {i+1}: {str(ve)}")
                    
                except Exception as chunk_e:
                    logger.error(f"Error processing chunk {i+1} from file {file_id}: {str(chunk_e)}")
                    import traceback
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
                import traceback
                logger.error(f"Exception details: {traceback.format_exc()}")

def chunk_text(text: str, chunk_size: int, chunk_overlap: int) -> List[str]:
    """
    Split text into overlapping chunks using LangChain's RecursiveCharacterTextSplitter.
    
    Args:
        text: The text to split
        chunk_size: Maximum size of each chunk (in characters)
        chunk_overlap: Overlap between chunks (in characters)
    
    Returns:
        List[str]: List of text chunks
    """
    try:
        # Import langchain text splitter
        from langchain_text_splitters import RecursiveCharacterTextSplitter
        
        logger.info(f"Using LangChain's RecursiveCharacterTextSplitter with chunk_size={chunk_size}, chunk_overlap={chunk_overlap}")
        
        # Create text splitter with appropriate parameters
        text_splitter = RecursiveCharacterTextSplitter(
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            length_function=len,
            is_separator_regex=False,
            separators=["\n\n", "\n", " ", ""]
        )
        
        # Split text
        chunks = text_splitter.split_text(text)
        
        # Log chunk information
        logger.info(f"Split text into {len(chunks)} chunks")
        chunk_lengths = [len(chunk) for chunk in chunks]
        if chunk_lengths:
            logger.debug(f"Chunk lengths - min: {min(chunk_lengths)}, max: {max(chunk_lengths)}, avg: {sum(chunk_lengths)/len(chunk_lengths):.1f}")
        
        return chunks
    except ImportError as e:
        # Fallback to original implementation if LangChain is not available
        logger.warning(f"LangChain import error: {str(e)}. Falling back to basic chunking method.")
        return _chunk_text_basic(text, chunk_size, chunk_overlap)
    except Exception as e:
        # Fallback if there are any other errors
        logger.error(f"Error using LangChain text splitter: {str(e)}. Falling back to basic chunking method.")
        import traceback
        logger.error(f"Traceback: {traceback.format_exc()}")
        return _chunk_text_basic(text, chunk_size, chunk_overlap)

def _chunk_text_basic(text: str, chunk_size: int, chunk_overlap: int) -> List[str]:
    """
    Basic fallback method to split text into overlapping chunks.
    
    Args:
        text: The text to split
        chunk_size: Maximum size of each chunk
        chunk_overlap: Overlap between chunks
    
    Returns:
        List[str]: List of text chunks
    """
    logger.info("Using basic chunking method")
    chunks = []
    start = 0
    text_length = len(text)
    
    while start < text_length:
        # Calculate end position with respect to text length
        end = min(start + chunk_size, text_length)
        
        # Try to end at a paragraph or sentence if possible
        if end < text_length:
            # Look for paragraph breaks first
            paragraph_end = text.rfind("\n\n", start, end)
            if paragraph_end != -1 and paragraph_end > start + chunk_size // 2:  # Ensure meaningful chunk size
                end = paragraph_end + 2  # Include the newlines
            else:
                # Look for sentence breaks
                sentence_end = text.rfind(". ", start, end)
                if sentence_end != -1 and sentence_end > start + chunk_size // 3:  # Ensure meaningful chunk size
                    end = sentence_end + 2  # Include the period and space
        
        # Extract chunk
        chunk = text[start:end]
        chunks.append(chunk)
        
        # Move starting position for next chunk, considering overlap
        start = start + chunk_size - chunk_overlap
        
        # Break if we've reached the end
        if start >= text_length:
            break
    
    # Log chunk information
    logger.info(f"Split text into {len(chunks)} chunks using basic method")
    chunk_lengths = [len(chunk) for chunk in chunks]
    if chunk_lengths:
        logger.debug(f"Chunk lengths - min: {min(chunk_lengths)}, max: {max(chunk_lengths)}, avg: {sum(chunk_lengths)/len(chunk_lengths):.1f}")
    
    return chunks

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

def prepare_context_for_llm(thread_id: str, query_message: Dict[str, Any]) -> Tuple[str, str, str]:
    """
    Prepare context for the LLM including query text, query context, and local context.
    
    Args:
        thread_id: The thread ID
        query_message: The query message object
        
    Returns:
        Tuple[str, str, str]: query_text, query_context_text, local_context_text
    """
    logger.info(f"Preparing context for LLM with query message {query_message.get('message_id', 'unknown')}")
    
    # Extract query text from the message
    query_text = ""
    if query_message.get("content", {}).get("type") == "text":
        query_text = query_message.get("content", {}).get("text", "")
    
    # Get query attachments content
    query_attachments = []
    file_ids = query_message.get("file_ids", [])
    
    for file_id in file_ids:
        try:
            file_data = get_file(file_id)
            if file_data and "content" in file_data:
                query_attachments.append(file_data["content"])
        except Exception as e:
            logger.error(f"Error retrieving file {file_id}: {str(e)}")
    
    query_context_text = "\n".join(query_attachments)
    
    # Retrieve relevant local context using embeddings
    local_context_text = retrieve_relevant_context(thread_id, query_text)
    
    # Log the original context components before any modification
    logger.debug("============ ORIGINAL CONTEXT COMPONENTS ============")
    logger.debug(f"Query Text (first 500 chars): {query_text[:500]}")
    logger.debug(f"Query Context (first 500 chars): {query_context_text[:500]}")
    logger.debug(f"Local Context (first 500 chars): {local_context_text[:500]}")
    logger.debug("===================================================")
    
    # Ensure everything fits within token limits
    query_text, query_context_text, local_context_text = build_llm_context(
        query_text=query_text,
        query_context=query_context_text,
        local_context=local_context_text,
        max_tokens=64000,  # DeepSeek Reasoner context window size
        provider="deepseek",
        model="deepseek-reasoner"
    )
    
    # Log the final context components after token fitting
    logger.debug("============ FINAL CONTEXT COMPONENTS AFTER TOKEN FITTING ============")
    logger.debug(f"Final Query Text (first 500 chars): {query_text[:500]}")
    logger.debug(f"Final Query Context (first 500 chars): {query_context_text[:500]}")
    logger.debug(f"Final Local Context (first 500 chars): {local_context_text[:500]}")
    logger.debug("===================================================================")
    
    return query_text, query_context_text, local_context_text

def retrieve_relevant_context(thread_id: str, query_text: str) -> str:
    """
    Retrieve relevant context from vector store based on query.
    
    Args:
        thread_id: The thread ID
        query_text: The query text
        
    Returns:
        str: Relevant context as a string
    """
    logger.info(f"Retrieving relevant context for thread {thread_id}")
    
    # Skip empty queries
    if not query_text or query_text.strip() == "":
        logger.warning("Empty query text provided, skipping context retrieval")
        return ""
    
    # Generate embedding for query
    try:
        embedding_list = embed_text(query_text)
        if not embedding_list:
            logger.warning("Embedding function returned None for query")
            return ""
    except Exception as e:
        logger.error(f"Error generating embedding for query: {str(e)}")
        return ""
    
    # Log embedding info
    logger.debug(f"Query embedding type: {type(embedding_list).__name__}, length: {len(embedding_list)}")
    
    # Get optimal top_k based on query
    try:
        top_k_result = get_optimal_top_k(query_text)
        top_k = top_k_result.get("top_k", 5)
        logger.info(f"Using top_k = {top_k} for context retrieval")
    except Exception as e:
        logger.warning(f"Error getting optimal top_k, using default: {str(e)}")
        top_k = 5
    
    # Search for relevant vectors with better error handling
    try:
        similar_vectors = search_vectors(
            query_embedding=embedding_list,
            namespace="files",
            thread_id=thread_id,
            similarity_threshold=0.5,
            limit=top_k
        )
        
        # Check if we got any vectors back
        if not similar_vectors:
            logger.info("No similar vectors found in database")
            return ""
            
        # Format retrieved contexts
        contexts = []
        for i, vector in enumerate(similar_vectors):
            content = vector.get("content", "")
            if content:
                source_info = ""
                metadata = vector.get("metadata", {})
                if metadata:
                    file_name = metadata.get("file_name", "")
                    if file_name:
                        source_info = f" (Source: {file_name})"
                
                contexts.append(f"Chunk #{i+1}{source_info}:\n{content}")
        
        logger.info(f"Retrieved {len(contexts)} relevant context chunks")
        return "\n\n".join(contexts)
    except Exception as e:
        logger.error(f"Error retrieving context: {str(e)}")
        import traceback
        logger.error(f"Traceback: {traceback.format_exc()}")
        return ""

def build_llm_context(
    query_text: str,
    query_context: str,
    local_context: str,
    max_tokens: int,
    provider: str,
    model: str
) -> Tuple[str, str, str]:
    """
    Build the context for the LLM, ensuring it fits within token limits.
    
    Args:
        query_text: The user query
        query_context: Context from query's attachments
        local_context: Retrieved context from vector store
        max_tokens: Maximum token limit
        provider: The model provider
        model: The model name
        
    Returns:
        Tuple[str, str, str]: Finalized query_text, query_context, local_context
    """
    from tools.tokenizer import token_counter
    
    logger.info("Building LLM context with token management")
    
    # Count tokens for each component
    query_text_tokens, _, _ = token_counter(query_text, provider, model)
    query_context_tokens, _, _ = token_counter(query_context, provider, model)
    local_context_tokens, _, _ = token_counter(local_context, provider, model)
    
    # Log token counts for each component
    logger.debug("============ TOKEN COUNTS FOR CONTEXT COMPONENTS ============")
    logger.debug(f"Query Text: {query_text_tokens} tokens")
    logger.debug(f"Query Context: {query_context_tokens} tokens")
    logger.debug(f"Local Context: {local_context_tokens} tokens")
    logger.debug(f"Total: {query_text_tokens + query_context_tokens + local_context_tokens} tokens (max: {max_tokens})")
    logger.debug("===========================================================")
    
    # Quick check if everything fits
    total_tokens = query_text_tokens + query_context_tokens + local_context_tokens
    if total_tokens <= max_tokens:
        logger.info(f"All content fits within token limit ({total_tokens}/{max_tokens})")
        return query_text, query_context, local_context
    
    logger.warning(f"Total tokens {total_tokens} exceeds limit {max_tokens}. Need to reduce content.")
    
    # If query alone exceeds the limit, summarize it
    if query_text_tokens > max_tokens:
        logger.info(f"Query text exceeds limit, summarizing ({query_text_tokens}/{max_tokens})")
        result = summarize_context(query_text, max_tokens, provider, model)
        if result["status"] == 200:
            query_text = result["content"]
            logger.debug(f"Summarized query from {query_text_tokens} to {token_counter(query_text, provider, model)[0]} tokens")
            return query_text, "", ""
    
    # Weighted distribution
    p_A = 2  # Query priority (lowered from 3 to 2)
    p_B = 2  # Query context priority
    p_C = 2  # Local context priority (increased from 1 to 2)
    W = p_A + p_B + p_C
    
    # Calculate capacity slices
    c_A = (p_A / W) * max_tokens
    c_B = (p_B / W) * max_tokens
    c_C = (p_C / W) * max_tokens
    
    logger.debug(f"Token allocation - Query: {int(c_A)}, Query Context: {int(c_B)}, Local Context: {int(c_C)}")
    
    # Try to fit query fully
    if query_text_tokens <= c_A:
        final_query = query_text
        leftover = c_A - query_text_tokens
        c_B += leftover
        logger.debug(f"Query fits within allocation. Leftover {int(leftover)} tokens added to Query Context (now {int(c_B)})")
    else:
        # Summarize query
        logger.info(f"Query exceeds its allocation, summarizing ({query_text_tokens}/{int(c_A)})")
        result = summarize_context(query_text, int(c_A), provider, model)
        if result["status"] == 200:
            final_query = result["content"]
            logger.debug(f"Summarized query from {query_text_tokens} to {token_counter(final_query, provider, model)[0]} tokens")
        else:
            # Fallback if summarization fails
            final_query = query_text[:int(len(query_text) * (c_A / query_text_tokens))]
            logger.warning(f"Summarization failed, truncated query to {token_counter(final_query, provider, model)[0]} tokens")
    
    # Try to fit query context fully
    if query_context_tokens <= c_B:
        final_query_context = query_context
        leftover = c_B - query_context_tokens
        c_C += leftover
        logger.debug(f"Query context fits within allocation. Leftover {int(leftover)} tokens added to Local Context (now {int(c_C)})")
    else:
        # Summarize query context
        logger.info(f"Query context exceeds its allocation, summarizing ({query_context_tokens}/{int(c_B)})")
        result = summarize_context(query_context, int(c_B), provider, model)
        if result["status"] == 200:
            final_query_context = result["content"]
            logger.debug(f"Summarized query context from {query_context_tokens} to {token_counter(final_query_context, provider, model)[0]} tokens")
        else:
            # Fallback if summarization fails
            final_query_context = query_context[:int(len(query_context) * (c_B / query_context_tokens))]
            logger.warning(f"Summarization failed, truncated query context to {token_counter(final_query_context, provider, model)[0]} tokens")
    
    # Fit local context
    if local_context_tokens <= c_C:
        final_local_context = local_context
        logger.debug(f"Local context fits within allocation ({local_context_tokens}/{int(c_C)})")
    else:
        # Summarize local context
        logger.info(f"Local context exceeds its allocation, summarizing ({local_context_tokens}/{int(c_C)})")
        result = summarize_context(local_context, int(c_C), provider, model)
        if result["status"] == 200:
            final_local_context = result["content"]
            logger.debug(f"Summarized local context from {local_context_tokens} to {token_counter(final_local_context, provider, model)[0]} tokens")
        else:
            # Fallback if summarization fails
            final_local_context = local_context[:int(len(local_context) * (c_C / local_context_tokens))]
            logger.warning(f"Summarization failed, truncated local context to {token_counter(final_local_context, provider, model)[0]} tokens")
    
    # Final token check
    final_total_tokens = token_counter(final_query, provider, model)[0] + token_counter(final_query_context, provider, model)[0] + token_counter(final_local_context, provider, model)[0]
    logger.info(f"Final context size after adjustments: {final_total_tokens}/{max_tokens} tokens")
    
    logger.info("Context building complete")
    return final_query, final_query_context, final_local_context

def generate_llm_response(
    query_text: str,
    query_context_text: str,
    local_context_text: str,
    model_name: str
) -> str:
    """
    Generate a response from the LLM using the prepared context.
    
    Args:
        query_text: The user query
        query_context_text: Context from query's attachments
        local_context_text: Retrieved context from vector store
        model_name: The model name
        
    Returns:
        str: The LLM's response
    """
    logger.info("Generating LLM response")
    
    # Check if API key is available
    if not DEEPSEEK_API_KEY:
        error_msg = "DeepSeek API key is not configured. Please set the DEEPSEEK_API_KEY environment variable."
        logger.error(error_msg)
        return f"I apologize, but I'm unable to process your request at the moment. {error_msg}"
    
    # Validate API key format
    if len(DEEPSEEK_API_KEY.strip()) < 10:  # Simple length check
        error_msg = "DeepSeek API key appears to be invalid (too short)."
        logger.error(error_msg)
        return f"I apologize, but I'm unable to process your request due to an API configuration issue. Please contact support."
    
    # Build system prompt with context
    system_prompt = "You are a helpful assistant. Use the information below to answer."
    
    if local_context_text:
        system_prompt += "\n\n[LOCAL DOCUMENT CONTEXT]\n" + local_context_text
    
    # Combine query and its context
    user_prompt = query_text
    if query_context_text:
        user_prompt += "\n\n[QUERY CONTEXT]\n" + query_context_text
    
    # Log detailed message construction
    logger.debug("---------- DETAILED MESSAGE CONSTRUCTION ----------")
    logger.debug(f"SYSTEM MESSAGE:")
    logger.debug(f"Base system prompt: 'You are a helpful assistant. Use the information below to answer.'")
    if local_context_text:
        logger.debug(f"Added local context of {len(local_context_text)} chars with marker '[LOCAL DOCUMENT CONTEXT]'")
    else:
        logger.debug("No local context added to system message")
    
    logger.debug(f"USER MESSAGE:")
    logger.debug(f"Base query: '{query_text[:200]}...' ({len(query_text)} chars)")
    if query_context_text:
        logger.debug(f"Added query context of {len(query_context_text)} chars with marker '[QUERY CONTEXT]'")
    else:
        logger.debug("No query context added to user message")
    logger.debug("---------------------------------------------------")
    
    # Log prompt details (lengths only, not content)
    logger.debug(f"System prompt length: {len(system_prompt)} chars")
    logger.debug(f"User prompt length: {len(user_prompt)} chars")
    
    # Generate response using DeepSeek API
    try:
        # Prepare request data
        request_data = {
            "model": "deepseek-reasoner",
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt}
            ],
            "temperature": 0.6,
            "max_tokens": 8000
        }
        
        # Log detailed message structure with content
        logger.debug("---------- COMPLETE MESSAGE STRUCTURE SENT TO API ----------")
        for i, msg in enumerate(request_data["messages"]):
            role = msg["role"]
            content = msg["content"]
            logger.debug(f"Message {i+1} - Role: {role}")
            # Split content into chunks for more readable logs
            if len(content) > 2000:
                chunks = [content[i:i+2000] for i in range(0, len(content), 2000)]
                for j, chunk in enumerate(chunks):
                    logger.debug(f"Message {i+1} Content (Part {j+1}/{len(chunks)}): {chunk}")
            else:
                logger.debug(f"Message {i+1} Content: {content}")
        logger.debug("----------------------------------------------------------")
        
        # Add a high-level log message for quick scanning
        logger.info(f"Sending DeepSeek API request for model: {request_data['model']}, temperature: {request_data['temperature']}, max_tokens: {request_data['max_tokens']}")
        
        # Set up headers with auth token
        headers = {
            "Authorization": f"Bearer {DEEPSEEK_API_KEY}",
            "Content-Type": "application/json"
        }
        
        # Make API request
        logger.info(f"Sending request to DeepSeek Reasoner API at {DEEPSEEK_API_URL}")
        response = requests.post(
            DEEPSEEK_API_URL,
            headers=headers,
            json=request_data
        )
        
        # Check if the request was successful
        if response.status_code == 200:
            response_json = response.json()
            logger.debug(f"API response structure: {json.dumps({k: type(v).__name__ for k, v in response_json.items()})}")
            if "choices" in response_json and response_json["choices"]:
                response_text = response_json["choices"][0]["message"]["content"]
                if response_text:
                    logger.info("Successfully generated LLM response")
                    return response_text
                else:
                    logger.warning("Empty response text from API")
                    return "I apologize, but I received an empty response from the language model. Please try again with a more specific query."
            else:
                logger.warning(f"Unexpected API response structure: {response_json}")
                return "I apologize, but I received an unexpected response format from the language model. Please try again later."
        else:
            error_message = f"API error: {response.status_code} - {response.text}"
            logger.error(error_message)
            
            # Handle specific error codes
            if response.status_code == 401:
                return ("I apologize, but there seems to be an authentication issue with the DeepSeek service. "
                        "Please check your API key configuration and try again later.")
            elif response.status_code == 429:
                return ("I apologize, but the DeepSeek service is currently rate limited. "
                        "Please try again after a short while.")
            else:
                return f"I apologize, but I encountered an error processing your request. Please try again later."
                
    except Exception as e:
        logger.error(f"Error generating LLM response: {str(e)}")
        
        # Check for connection errors
        error_str = str(e).lower()
        if "timeout" in error_str or "connection" in error_str:
            return ("I apologize, but I encountered a connection issue with the DeepSeek service. "
                    "Please check your internet connection and try again.")
        else:
            # Add more detailed logging for unexpected errors
            import traceback
            logger.error(f"Unexpected error details: {traceback.format_exc()}")
            return f"I apologize, but I encountered an error processing your request. Please try again later."

def store_assistant_response(thread_id: str, content: str, user_author: Dict[str, Any]) -> Dict[str, Any]:
    """
    Store the assistant's response and process for vector storage.
    
    Args:
        thread_id: The thread ID
        content: The response content
        user_author: The user author information (for context)
        
    Returns:
        Dict[str, Any]: The created message data
    """
    logger.info(f"Storing assistant response for thread {thread_id}")
    
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
        
        # Process response for vector storage only if significant content
        if len(content) > 10:  # Skip vectorization for very short responses
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
                        namespace="messages",
                        embed_tool={"type": "embed", "model": "gemini-embedding-exp-03-07"}
                    )
                    logger.info("Successfully stored assistant response vector")
                else:
                    logger.warning("Failed to generate embedding for assistant response")
            except Exception as vector_e:
                # Handle embedding/vector storage errors but still return the created message
                logger.error(f"Error storing vector for assistant response: {str(vector_e)}")
                logger.error(f"Content that failed to embed: {content[:100]}...")
                import traceback
                logger.error(f"Vector error traceback: {traceback.format_exc()}")
        else:
            logger.info("Skipping vectorization for short assistant response")
        
        return created_message
    except Exception as e:
        logger.error(f"Error storing assistant response: {str(e)}")
        import traceback
        logger.error(f"Error traceback: {traceback.format_exc()}")
        raise

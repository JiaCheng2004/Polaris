# models/deepseek/deepseek_chat.py
"""
DeepSeek Chat integration module.

This module implements the DeepSeek Chat API with the model-agnostic document toolkit.
"""

import uuid
import json
import requests
import traceback
from typing import Any, Dict, List, Optional

# Import logger
from tools.logger import logger
from tools.config.load import DEEPSEEK_API_KEY

# Import document toolkit functions
from tools.docs.thread_utils import handle_thread_management
from tools.docs.message_utils import process_incoming_messages, get_most_recent_user_query, store_assistant_response
from tools.docs.context_utils import prepare_context_for_llm, retrieve_relevant_context, build_llm_context

# Import search functionality
from tools.search import unified_search
from tools.llm.search_indicator import detect_search_needs

# DeepSeek API endpoint
DEEPSEEK_API_URL = "https://api.deepseek.com/v1/chat/completions"

def generate_deepseek_chat_response(
    query_text: str,
    query_context_text: str,
    local_context_text: str,
    model_name: str,
    temperature: float = 0.7,
    max_output_tokens: int = 8000
) -> str:
    """
    Generate a response from the DeepSeek Chat API.
    
    Args:
        query_text: The user query
        query_context_text: Context from query's attachments
        local_context_text: Retrieved context from vector store
        model_name: The model name (should be "deepseek-chat")
        temperature: Temperature for response generation
        max_output_tokens: Maximum tokens to generate in the response
        
    Returns:
        str: The generated response
    """
    logger.info("Generating DeepSeek Chat response")
    
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
    
    # Log prompt details
    logger.debug(f"System prompt length: {len(system_prompt)} chars")
    logger.debug(f"User prompt length: {len(user_prompt)} chars")
    
    # Generate response using DeepSeek API
    try:
        # Prepare request data
        request_data = {
            "model": model_name,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt}
            ],
            "temperature": temperature,
            "max_tokens": max_output_tokens
        }
        
        # Set up headers with auth token
        headers = {
            "Authorization": f"Bearer {DEEPSEEK_API_KEY}",
            "Content-Type": "application/json"
        }
        
        # Make API request
        logger.info(f"Sending request to DeepSeek API at {DEEPSEEK_API_URL}")
        response = requests.post(
            DEEPSEEK_API_URL,
            headers=headers,
            json=request_data
        )
        
        # Check if the request was successful
        if response.status_code == 200:
            response_json = response.json()
            if "choices" in response_json and response_json["choices"]:
                response_text = response_json["choices"][0]["message"]["content"]
                if response_text:
                    logger.info("Successfully generated response")
                    return response_text
                else:
                    logger.warning("Empty response text from API")
                    return "I apologize, but I received an empty response. Please try again with a more specific query."
            else:
                logger.warning(f"Unexpected API response structure: {response_json}")
                return "I apologize, but I received an unexpected response format. Please try again later."
        else:
            error_message = f"API error: {response.status_code} - {response.text}"
            logger.error(error_message)
            
            # Handle specific error codes
            if response.status_code == 401:
                return "I apologize, but there seems to be an authentication issue. Please check API key configuration."
            elif response.status_code == 429:
                return "I apologize, but the service is currently rate limited. Please try again after a short while."
            else:
                return f"I apologize, but I encountered an error processing your request. Please try again later."
                
    except Exception as e:
        logger.error(f"Error generating response: {str(e)}")
        logger.error(f"Traceback: {traceback.format_exc()}")
        return f"I apologize, but I encountered an error processing your request. Please try again later."

def create_deepseek_chat_completion(payload: Dict[str, Any], files: Optional[List[Any]] = None) -> Dict[str, Any]:
    """
    Main function to handle chat completions with DeepSeek Chat.
    
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
    logger.info("Starting DeepSeek Chat completion")
    
    try:
        # Extract fields from payload
        provider = payload.get("provider", "deepseek")
        model_name = payload.get("model", "deepseek-chat")
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
            # DeepSeek-specific configuration
            embedding_model = {"type": "embed", "model": "gemini-embedding-exp-03-07"}
            
            processed_messages = process_incoming_messages(
                thread_id=thread_id, 
                messages=messages, 
                author=author, 
                embedding_model=embedding_model
            )
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
            # DeepSeek Chat-specific context configuration
            query_text, query_context_text, local_context_text = prepare_context_for_llm(
                thread_id=thread_id,
                query_message=query_message,
                max_tokens=64000,  # DeepSeek Chat context window size
                provider="deepseek",
                model="deepseek-chat",
                use_summarization=True
            )
            logger.debug(f"Prepared context - Query: {len(query_text)} chars, Query Context: {len(query_context_text)} chars, Local Context: {len(local_context_text)} chars")
        except Exception as context_e:
            logger.error(f"Error preparing context: {str(context_e)}")
            query_text = query_message.get("content", {}).get("text", "")
            query_context_text = ""
            local_context_text = ""
            logger.info("Using fallback context (query text only)")
        
        # Generate response from LLM
        try:
            response = generate_deepseek_chat_response(
                query_text=query_text,
                query_context_text=query_context_text,
                local_context_text=local_context_text,
                model_name=model_name
            )
            logger.debug(f"Generated response: {len(response)} chars")
        except Exception as llm_e:
            logger.error(f"Error generating response: {str(llm_e)}")
            response = f"I apologize, but I encountered an error processing your request: {str(llm_e)}"
        
        # Store assistant's response
        try:
            # DeepSeek-specific vectorization configuration
            embedding_model = {"type": "embed", "model": "gemini-embedding-exp-03-07"}
            
            response_message = store_assistant_response(
                thread_id=thread_id, 
                content=response, 
                user_author=author,
                embedding_model=embedding_model
            )
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
        logger.error(f"Unexpected error in deepseek_chat_completion: {str(e)}")
        logger.error(f"Traceback: {traceback.format_exc()}")
        return {
            "error": "An unexpected error occurred processing your request",
            "content": "I apologize, but I encountered an unexpected error processing your request. Please try again later."
        }

def create_deepseek_reasoner_completion(payload: Dict[str, Any], files: Optional[List[Any]] = None) -> Dict[str, Any]:
    """
    Main function to handle chat completions with DeepSeek Reasoner.
    
    Implements:
    1) Thread management (create new thread or use existing)
    2) Process files and store in vector database
    3) Analysis of search needs for external information
    4) Retrieval of relevant documents for context
    5) Context management to fit into model's token limit
    6) LLM response generation
    
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
            # DeepSeek-specific configuration
            embedding_model = {"type": "embed", "model": "gemini-embedding-exp-03-07"}
            
            processed_messages = process_incoming_messages(
                thread_id=thread_id, 
                messages=messages, 
                author=author, 
                embedding_model=embedding_model
            )
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
        
        # Extract query text from the message
        query_text = ""
        if query_message.get("content", {}).get("type") == "text":
            query_text = query_message.get("content", {}).get("text", "")
        
        # Get attachments content
        query_attachments = []
        file_ids = query_message.get("file_ids", [])
        
        for file_id in file_ids:
            try:
                from tools.database.file import get_file
                file_data = get_file(file_id)
                if file_data and "content" in file_data:
                    query_attachments.append(f"[File: {file_data.get('filename', file_id)}]\n{file_data['content']}")
            except Exception as e:
                logger.error(f"Error retrieving file {file_id}: {str(e)}")
        
        query_context_text = "\n\n".join(query_attachments) if query_attachments else ""
        
        # Check if search is needed using search indicator
        search_results_text = ""
        try:
            search_needs = detect_search_needs(query_text)
            logger.info(f"Search indicator results: {search_needs}")
            
            # Modified condition: Check for tool key OR web_search key
            if (search_needs and 
                (search_needs.get("tool") or search_needs.get("web_search") or 
                search_needs.get("videos") or search_needs.get("web_scrap"))):
                
                logger.info("Search is needed based on query analysis")
                
                # Get the search query - use web_search value if available, otherwise use original query
                search_query = search_needs.get("web_search", query_text)
                logger.info(f"Performing search with query: {search_query}")
                
                search_results_text = unified_search(search_query)
                
                # Check if search was successful and returned content
                if search_results_text:
                    logger.info(f"Retrieved search results: {len(search_results_text)} chars")
                    logger.debug(f"Search results preview: {search_results_text[:500]}...")
                    
                    # Check if the results actually contain data or just the "No results found" message
                    if len(search_results_text) > 100 and "No results found" not in search_results_text:
                        # Add search results to query context
                        if query_context_text:
                            query_context_text += "\n\n" + search_results_text
                        else:
                            query_context_text = search_results_text
                        
                        logger.info("Successfully integrated search results into context")
                    else:
                        logger.warning(f"Search results too short or contain 'No results': {search_results_text}")
                        if query_context_text:
                            query_context_text += "\n\nNote: Searched for information but no relevant results were found."
                else:
                    logger.warning("Search returned empty results")
                    if query_context_text:
                        query_context_text += "\n\nNote: Attempted to search for information but no results were found."
            else:
                logger.info("No search needed based on query analysis")
        except Exception as search_e:
            logger.error(f"Error during search processing: {str(search_e)}")
            logger.error(f"Search error traceback: {traceback.format_exc()}")
            # Proceed without search results, but add a note about the failure
            if query_context_text:
                query_context_text += "\n\nNote: Attempted to search for additional information but encountered an error."
            # Continue without search results
        
        # Retrieve local context from vector store
        local_context_text = ""
        try:
            local_context_text = retrieve_relevant_context(
                thread_id=thread_id, 
                query_text=query_text,
                namespace="files"
            )
            logger.info(f"Retrieved local context: {len(local_context_text)} chars")
        except Exception as context_e:
            logger.error(f"Error retrieving local context: {str(context_e)}")
            # Proceed without local context
        
        # Ensure everything fits within token limits
        try:
            # DeepSeek Reasoner context window size
            max_tokens = 64000
            
            query_text, query_context_text, local_context_text = build_llm_context(
                query_text=query_text,
                query_context=query_context_text,
                local_context=local_context_text,
                max_tokens=max_tokens,
                provider="deepseek",
                model="deepseek-reasoner",
                use_summarization=True
            )
            logger.info("Successfully built context for LLM")
        except Exception as token_e:
            logger.error(f"Error building LLM context: {str(token_e)}")
            logger.error(f"Context error traceback: {traceback.format_exc()}")
            # Use what we have so far and proceed
        
        # Generate response from LLM
        try:
            response = generate_deepseek_chat_response(
                query_text=query_text,
                query_context_text=query_context_text,
                local_context_text=local_context_text,
                model_name=model_name
            )
            logger.debug(f"Generated response: {len(response)} chars")
        except Exception as llm_e:
            logger.error(f"Error generating response: {str(llm_e)}")
            response = f"I apologize, but I encountered an error processing your request: {str(llm_e)}"
        
        # Store assistant's response
        try:
            # DeepSeek-specific vectorization configuration
            embedding_model = {"type": "embed", "model": "gemini-embedding-exp-03-07"}
            
            response_message = store_assistant_response(
                thread_id=thread_id, 
                content=response, 
                user_author=author,
                embedding_model=embedding_model
            )
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
        logger.error(f"Traceback: {traceback.format_exc()}")
        return {
            "error": "An unexpected error occurred processing your request",
            "content": "I apologize, but I encountered an unexpected error processing your request. Please try again later."
        }
